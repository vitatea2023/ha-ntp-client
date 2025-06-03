package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/facebook/time/ntp/protocol"
	dnsclient "github.com/vitatea2023/ha-dns-client"
)

// NTPResult represents the result of an NTP query
type NTPResult struct {
	Server    NTPServerConfig `json:"server"`
	IP        string          `json:"ip"`
	Offset    time.Duration   `json:"offset"`
	Delay     time.Duration   `json:"delay"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Success   bool            `json:"success"`
}

// NTPTestResults represents the complete test results
type NTPTestResults struct {
	TestTime    time.Time   `json:"test_time"`
	TotalTests  int         `json:"total_tests"`
	Successful  int         `json:"successful"`
	Failed      int         `json:"failed"`
	BestServer  *NTPResult  `json:"best_server,omitempty"`
	AllResults  []NTPResult `json:"all_results"`
	Summary     string      `json:"summary"`
}

// HANTPClient is the high-availability NTP client
type HANTPClient struct {
	config    *Config
	dnsClient *dnsclient.DNSClient
}

// NewHANTPClient creates a new HA NTP client
func NewHANTPClient(config *Config) *HANTPClient {
	var dnsClient *dnsclient.DNSClient
	if config.UseHADNS {
		dnsClient = dnsclient.NewDNSClient()
	}
	
	return &HANTPClient{
		config:    config,
		dnsClient: dnsClient,
	}
}

// resolveServerIP resolves NTP server hostname to IP using HA DNS or system DNS
func (c *HANTPClient) resolveServerIP(hostname string) (string, error) {
	// If it's already an IP address, return as-is
	if net.ParseIP(hostname) != nil {
		return hostname, nil
	}
	
	// Use HA DNS client if enabled (but without health check - just use DNS resolution)
	if c.config.UseHADNS && c.dnsClient != nil {
		log.Printf("🔍 Resolving %s using HA DNS client...", hostname)
		
		// Try DoH first, then UDP DNS, then system DNS directly without health checks
		if ip, err := c.resolveWithDOHOnly(hostname); err == nil {
			log.Printf("✅ HA DNS (DoH) resolved %s -> %s", hostname, ip)
			return ip, nil
		}
		
		if ip, err := c.resolveWithUDPOnly(hostname); err == nil {
			log.Printf("✅ HA DNS (UDP) resolved %s -> %s", hostname, ip)
			return ip, nil
		}
		
		log.Printf("⚠️ HA DNS failed for %s, falling back to system DNS", hostname)
	}
	
	// Fallback to system DNS
	log.Printf("🔍 Resolving %s using system DNS...", hostname)
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DNSTimeout)
	defer cancel()
	
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", fmt.Errorf("DNS resolution failed: %w", err)
	}
	
	// Return first IPv4 address
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			return ip.IP.String(), nil
		}
	}
	
	return "", fmt.Errorf("no IPv4 address found for %s", hostname)
}

// resolveWithDOHOnly tries DoH resolution without health checks
func (c *HANTPClient) resolveWithDOHOnly(hostname string) (string, error) {
	// Use system DNS to resolve DoH servers first, then query them
	dohServers := []string{"223.5.5.5", "223.6.6.6", "1.12.12.12", "120.53.53.53"}
	
	for _, server := range dohServers {
		if ip, err := c.queryDNSServer(hostname, server); err == nil {
			return ip, nil
		}
	}
	
	return "", fmt.Errorf("DoH resolution failed")
}

// resolveWithUDPOnly tries UDP DNS resolution without health checks
func (c *HANTPClient) resolveWithUDPOnly(hostname string) (string, error) {
	udpServers := []string{
		"223.5.5.5", "223.6.6.6", "119.29.29.29", "182.254.116.116",
		"114.114.114.114", "114.114.115.115", "8.8.8.8", "1.1.1.1",
	}
	
	for _, server := range udpServers {
		if ip, err := c.queryDNSServer(hostname, server); err == nil {
			return ip, nil
		}
	}
	
	return "", fmt.Errorf("UDP DNS resolution failed")
}

// queryDNSServer performs a simple DNS A record query
func (c *HANTPClient) queryDNSServer(hostname, dnsServer string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.DNSTimeout)
	defer cancel()
	
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: c.config.DNSTimeout}
			return d.DialContext(ctx, network, dnsServer+":53")
		},
	}
	
	ips, err := r.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", err
	}
	
	// Return first IPv4 address
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			return ip.IP.String(), nil
		}
	}
	
	return "", fmt.Errorf("no IPv4 address found")
}

// validateNTPServerResponse validates NTP server response format
func (c *HANTPClient) validateNTPServerResponse(response *protocol.Packet) bool {
	settings := response.Settings
	li := settings >> 6           // Leap Indicator (2 bits)
	vn := (settings << 2) >> 5    // Version Number (3 bits)
	mode := (settings << 5) >> 5  // Mode (3 bits)
	
	// LI: must be 0 (no warning), 1 (last minute has 61 seconds), 2 (last minute has 59 seconds), or 3 (alarm condition)
	if li > 3 {
		return false
	}
	
	// VN: must be 1, 2, 3, or 4
	if vn < 1 || vn > 4 {
		return false
	}
	
	// Mode: must be 4 (server mode) for NTP server responses
	if mode != 4 {
		return false
	}
	
	return true
}

// queryNTPServer performs an NTP query to a specific server
func (c *HANTPClient) queryNTPServer(server NTPServerConfig, ip string) *NTPResult {
	result := &NTPResult{
		Server:    server,
		IP:        ip,
		Timestamp: time.Now(),
		Success:   false,
	}
	
	// Connect to NTP server
	serverAddr := fmt.Sprintf("%s:%d", ip, server.Port)
	conn, err := net.DialTimeout("udp", serverAddr, c.config.NTPTimeout)
	if err != nil {
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return result
	}
	defer conn.Close()
	
	// Set deadline for the entire operation
	deadline := time.Now().Add(c.config.NTPTimeout)
	conn.SetDeadline(deadline)
	
	// Create NTP request packet
	originTime := time.Now()
	originSec, originFrac := protocol.Time(originTime)
	
	request := &protocol.Packet{
		Settings:       0x1B, // LI=0, VN=3, Mode=3 (client)
		Stratum:        0,
		Poll:           4,    // 16 seconds
		Precision:      -6,   // ~15.6ms precision
		RootDelay:      0,
		RootDispersion: 0,
		ReferenceID:    0,
		RefTimeSec:     0,
		RefTimeFrac:    0,
		OrigTimeSec:    originSec,
		OrigTimeFrac:   originFrac,
		RxTimeSec:      0,
		RxTimeFrac:     0,
		TxTimeSec:      originSec,
		TxTimeFrac:     originFrac,
	}
	
	// Send request
	requestBytes, err := request.Bytes()
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result
	}
	
	_, err = conn.Write(requestBytes)
	if err != nil {
		result.Error = fmt.Sprintf("failed to send request: %v", err)
		return result
	}
	
	// Read response
	responseBytes := make([]byte, protocol.PacketSizeBytes)
	_, err = conn.Read(responseBytes)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read response: %v", err)
		return result
	}
	
	clientReceiveTime := time.Now()
	
	// Parse response
	response := &protocol.Packet{}
	err = response.UnmarshalBinary(responseBytes)
	if err != nil {
		result.Error = fmt.Sprintf("failed to parse response: %v", err)
		return result
	}
	
	// Validate response - NTP servers respond with mode 4 (server mode)
	if !c.validateNTPServerResponse(response) {
		result.Error = fmt.Sprintf("invalid server response format (settings=0x%02x)", response.Settings)
		return result
	}
	
	if response.Stratum == 0 {
		result.Error = "server not synchronized (stratum 0)"
		return result
	}
	
	if response.Stratum >= 16 {
		result.Error = fmt.Sprintf("server stratum too high: %d", response.Stratum)
		return result
	}
	
	// Calculate timestamps
	serverReceiveTime := protocol.Unix(response.RxTimeSec, response.RxTimeFrac)
	serverTransmitTime := protocol.Unix(response.TxTimeSec, response.TxTimeFrac)
	
	// Calculate offset and delay
	offset := protocol.Offset(originTime, serverReceiveTime, serverTransmitTime, clientReceiveTime)
	delay := protocol.RoundTripDelay(originTime, serverReceiveTime, serverTransmitTime, clientReceiveTime)
	
	result.Offset = time.Duration(offset)
	result.Delay = time.Duration(delay)
	result.Success = true
	
	return result
}

// TestServer tests a single NTP server
func (c *HANTPClient) TestServer(server NTPServerConfig) *NTPResult {
	log.Printf("🧪 Testing NTP server: %s (weight: %d, location: %s)", 
		server.Host, server.Weight, server.Location)
	
	// Resolve IP address
	ip, err := c.resolveServerIP(server.Host)
	if err != nil {
		log.Printf("❌ DNS resolution failed for %s: %v", server.Host, err)
		return &NTPResult{
			Server:    server,
			IP:        "",
			Error:     fmt.Sprintf("DNS resolution failed: %v", err),
			Timestamp: time.Now(),
			Success:   false,
		}
	}
	
	log.Printf("🔍 Resolved %s -> %s", server.Host, ip)
	
	// Try multiple times with retries
	var lastResult *NTPResult
	for attempt := 1; attempt <= c.config.MaxRetries; attempt++ {
		log.Printf("📡 Attempt %d/%d: Querying %s (%s)...", 
			attempt, c.config.MaxRetries, server.Host, ip)
		
		result := c.queryNTPServer(server, ip)
		
		if result.Success {
			log.Printf("✅ Success! Offset: %v, Delay: %v", 
				result.Offset, result.Delay)
			return result
		}
		
		log.Printf("❌ Attempt %d failed: %s", attempt, result.Error)
		lastResult = result
		
		if attempt < c.config.MaxRetries {
			time.Sleep(time.Second)
		}
	}
	
	log.Printf("💥 All %d attempts failed for %s", c.config.MaxRetries, server.Host)
	return lastResult
}

// TestAllServers tests all enabled NTP servers
func (c *HANTPClient) TestAllServers() *NTPTestResults {
	log.Printf("🚀 Starting NTP server testing...")
	
	servers := c.config.GetAllEnabledServers()
	if len(servers) == 0 {
		log.Printf("⚠️ No enabled servers found")
		return &NTPTestResults{
			TestTime:   time.Now(),
			TotalTests: 0,
			Summary:    "No enabled servers found",
		}
	}
	
	log.Printf("📋 Testing %d servers...", len(servers))
	
	// Use channels for concurrent testing
	resultChan := make(chan *NTPResult, len(servers))
	semaphore := make(chan struct{}, c.config.ParallelQueries)
	
	var wg sync.WaitGroup
	
	// Test servers concurrently
	for _, server := range servers {
		wg.Add(1)
		go func(srv NTPServerConfig) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			result := c.TestServer(srv)
			resultChan <- result
		}(server)
	}
	
	// Wait for all tests to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	
	// Collect results
	var allResults []NTPResult
	for result := range resultChan {
		allResults = append(allResults, *result)
	}
	
	// Sort results by success first, then by absolute offset
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].Success != allResults[j].Success {
			return allResults[i].Success // Successful results first
		}
		if !allResults[i].Success {
			return false // Both failed, keep original order
		}
		
		// Both successful, sort by absolute offset (lower is better)
		offsetI := allResults[i].Offset
		if offsetI < 0 {
			offsetI = -offsetI
		}
		offsetJ := allResults[j].Offset
		if offsetJ < 0 {
			offsetJ = -offsetJ
		}
		return offsetI < offsetJ
	})
	
	// Calculate statistics
	successful := 0
	for _, result := range allResults {
		if result.Success {
			successful++
		}
	}
	
	testResults := &NTPTestResults{
		TestTime:   time.Now(),
		TotalTests: len(allResults),
		Successful: successful,
		Failed:     len(allResults) - successful,
		AllResults: allResults,
	}
	
	// Find best server
	if successful > 0 {
		testResults.BestServer = &allResults[0]
		testResults.Summary = fmt.Sprintf("Best server: %s (%s) with offset %v", 
			allResults[0].Server.Host, allResults[0].IP, allResults[0].Offset)
	} else {
		testResults.Summary = "No servers responded successfully"
	}
	
	log.Printf("📊 Test completed: %d/%d servers successful", successful, len(allResults))
	if testResults.BestServer != nil {
		log.Printf("🏆 Best server: %s with offset %v", 
			testResults.BestServer.Server.Host, testResults.BestServer.Offset)
	}
	
	return testResults
}

// SaveTestResults saves test results to a file
func (c *HANTPClient) SaveTestResults(results *NTPTestResults, filename string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}
	
	log.Printf("💾 Test results saved to: %s", filename)
	return nil
}

// setSystemTime sets the system clock to the specified time (Linux/Unix only)
func setSystemTime(newTime time.Time) error {
	// Convert to Unix timestamp
	sec := newTime.Unix()
	nsec := newTime.UnixNano() % 1e9
	
	// Use syscall to set system time
	tv := syscall.Timeval{
		Sec:  sec,
		Usec: nsec / 1000, // Convert nanoseconds to microseconds
	}
	
	err := syscall.Settimeofday(&tv)
	if err != nil {
		return fmt.Errorf("settimeofday syscall failed: %w", err)
	}
	
	return nil
}

// adjustSystemClock adjusts the system clock by the specified offset using adjtimex
func adjustSystemClock(offset time.Duration) error {
	// Use adjtimex for gradual time adjustment (Linux only)
	type timex struct {
		modes     uint32
		offset    int64
		freq      int64
		maxerror  int64
		esterror  int64
		status    int32
		constant  int64
		precision int64
		tolerance int64
		time      syscall.Timeval
		tick      int64
		ppsfreq   int64
		jitter    int64
		shift     int32
		stabil    int64
		jitcnt    int64
		calcnt    int64
		errcnt    int64
		stbcnt    int64
		tai       int32
		_         [44]byte // padding
	}
	
	const ADJ_OFFSET = 0x0001
	const ADJ_NANO = 0x2000
	
	tx := timex{
		modes:  ADJ_OFFSET | ADJ_NANO,
		offset: offset.Nanoseconds(),
	}
	
	_, _, errno := syscall.Syscall(syscall.SYS_ADJTIMEX, uintptr(unsafe.Pointer(&tx)), 0, 0)
	if errno != 0 {
		return fmt.Errorf("adjtimex syscall failed: %v", errno)
	}
	
	return nil
}

// syncTimeWithCommand uses external command to set time (fallback method)
func syncTimeWithCommand(offset time.Duration) error {
	// Try different time setting commands
	commands := [][]string{
		{"timedatectl", "set-time", time.Now().Add(offset).Format("2006-01-02 15:04:05")},
		{"date", "-s", time.Now().Add(offset).Format("2006-01-02 15:04:05")},
	}
	
	for _, cmd := range commands {
		if _, err := exec.LookPath(cmd[0]); err == nil {
			log.Printf("🔧 Using %s to set time...", cmd[0])
			execCmd := exec.Command(cmd[0], cmd[1:]...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				log.Printf("⚠️ Command %s failed: %v, output: %s", cmd[0], err, string(output))
				continue
			}
			log.Printf("✅ Time set successfully using %s", cmd[0])
			return nil
		}
	}
	
	return fmt.Errorf("no suitable time setting command found")
}

// SyncTimeFromBestServer synchronizes system time using the best available server
func (c *HANTPClient) SyncTimeFromBestServer() error {
	log.Printf("🕐 Starting time synchronization...")
	
	results := c.TestAllServers()
	if results.BestServer == nil {
		return fmt.Errorf("no working NTP servers found")
	}
	
	bestResult := results.BestServer
	
	// Check if offset is within threshold
	absOffset := bestResult.Offset
	if absOffset < 0 {
		absOffset = -absOffset
	}
	
	if absOffset <= c.config.SyncThreshold {
		log.Printf("✅ System time is already synchronized (offset: %v <= threshold: %v)", 
			bestResult.Offset, c.config.SyncThreshold)
		return nil
	}
	
	log.Printf("⚠️ System time needs adjustment:")
	log.Printf("   Current offset: %v", bestResult.Offset)
	log.Printf("   Threshold: %v", c.config.SyncThreshold)
	log.Printf("   Best server: %s (%s)", bestResult.Server.Host, bestResult.IP)
	
	// Check if running as root
	if os.Geteuid() != 0 {
		log.Printf("❌ Time synchronization requires root privileges")
		log.Printf("💡 Please run with: sudo ./ha-ntp-client sync")
		return fmt.Errorf("insufficient privileges: must run as root")
	}
	
	// Attempt different time setting methods
	log.Printf("⚡ Adjusting system clock by: %v", bestResult.Offset)
	
	// Method 1: Try gradual adjustment with adjtimex (preferred for small offsets)
	if absOffset < 1*time.Second {
		log.Printf("🔧 Using gradual time adjustment (adjtimex)...")
		err := adjustSystemClock(bestResult.Offset)
		if err == nil {
			log.Printf("✅ Time synchronized successfully using gradual adjustment")
			return nil
		}
		log.Printf("⚠️ Gradual adjustment failed: %v", err)
	}
	
	// Method 2: Try direct time setting with syscall
	log.Printf("🔧 Using direct time setting (settimeofday)...")
	newTime := time.Now().Add(bestResult.Offset)
	err := setSystemTime(newTime)
	if err == nil {
		log.Printf("✅ Time synchronized successfully using settimeofday")
		log.Printf("🕐 System time adjusted to: %s", newTime.Format("2006-01-02 15:04:05 MST"))
		return nil
	}
	log.Printf("⚠️ Direct time setting failed: %v", err)
	
	// Method 3: Try external commands as fallback
	log.Printf("🔧 Using external commands as fallback...")
	err = syncTimeWithCommand(bestResult.Offset)
	if err == nil {
		log.Printf("✅ Time synchronized successfully using external command")
		return nil
	}
	log.Printf("⚠️ External command failed: %v", err)
	
	return fmt.Errorf("all time synchronization methods failed")
}