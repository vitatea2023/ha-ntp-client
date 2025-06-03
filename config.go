package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// NTPServerConfig represents a single NTP server configuration
type NTPServerConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Weight   int    `json:"weight" yaml:"weight"`     // Higher weight = higher priority
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	Location string `json:"location" yaml:"location"` // e.g., "China", "Global"
}

// Config represents the HA NTP client configuration
type Config struct {
	// Built-in NTP servers (Aliyun China)
	BuiltinServers []NTPServerConfig `json:"builtin_servers" yaml:"builtin_servers"`
	
	// Additional servers from config file
	AdditionalServers []NTPServerConfig `json:"additional_servers" yaml:"additional_servers"`
	
	// Remote URL to fetch server list
	RemoteServerListURL string `json:"remote_server_list_url" yaml:"remote_server_list_url"`
	
	// DNS resolver configuration
	DNSTimeout    time.Duration `json:"dns_timeout" yaml:"dns_timeout"`
	UseHADNS      bool          `json:"use_ha_dns" yaml:"use_ha_dns"`
	
	// NTP client configuration
	NTPTimeout        time.Duration `json:"ntp_timeout" yaml:"ntp_timeout"`
	MaxRetries        int           `json:"max_retries" yaml:"max_retries"`
	ParallelQueries   int           `json:"parallel_queries" yaml:"parallel_queries"`
	SyncThreshold     time.Duration `json:"sync_threshold" yaml:"sync_threshold"`
	
	// Test mode configuration
	TestMode          bool          `json:"test_mode" yaml:"test_mode"`
	TestTimeout       time.Duration `json:"test_timeout" yaml:"test_timeout"`
	TestResultFile    string        `json:"test_result_file" yaml:"test_result_file"`
	
	// Daemon mode configuration
	SyncInterval      time.Duration `json:"sync_interval" yaml:"sync_interval"`
}

// LoadDefaultConfig creates a configuration with built-in Aliyun NTP servers
func LoadDefaultConfig() *Config {
	return &Config{
		BuiltinServers: []NTPServerConfig{
			{Host: "ntp1.aliyun.com", Port: 123, Weight: 100, Enabled: true, Location: "China"},
			{Host: "ntp2.aliyun.com", Port: 123, Weight: 100, Enabled: true, Location: "China"},
			{Host: "ntp3.aliyun.com", Port: 123, Weight: 100, Enabled: true, Location: "China"},
			{Host: "ntp4.aliyun.com", Port: 123, Weight: 90, Enabled: true, Location: "China"},
			{Host: "ntp5.aliyun.com", Port: 123, Weight: 90, Enabled: true, Location: "China"},
			{Host: "ntp6.aliyun.com", Port: 123, Weight: 90, Enabled: true, Location: "China"},
			{Host: "ntp7.aliyun.com", Port: 123, Weight: 90, Enabled: true, Location: "China"},
			// Backup global servers
			{Host: "pool.ntp.org", Port: 123, Weight: 50, Enabled: true, Location: "Global"},
			{Host: "time.google.com", Port: 123, Weight: 60, Enabled: true, Location: "Global"},
			{Host: "time.cloudflare.com", Port: 123, Weight: 60, Enabled: true, Location: "Global"},
		},
		AdditionalServers:   []NTPServerConfig{},
		RemoteServerListURL: "",
		DNSTimeout:          10 * time.Second,
		UseHADNS:            true,
		NTPTimeout:          5 * time.Second,
		MaxRetries:          3,
		ParallelQueries:     5,
		SyncThreshold:       100 * time.Millisecond,
		TestMode:            false,
		TestTimeout:         30 * time.Second,
		TestResultFile:      "ntp_test_results.json",
		SyncInterval:        300 * time.Second, // 5分钟
	}
}

// LoadConfigFromFile loads configuration from a file (JSON or YAML)
func LoadConfigFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := LoadDefaultConfig()

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, config); err != nil {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file as JSON or YAML: %w", err)
		}
	}

	return config, nil
}

// LoadServersFromRemoteURL fetches NTP server list from remote URL
func (c *Config) LoadServersFromRemoteURL() error {
	if c.RemoteServerListURL == "" {
		return nil // No remote URL configured
	}

	client := &http.Client{
		Timeout: c.DNSTimeout,
	}

	resp, err := client.Get(c.RemoteServerListURL)
	if err != nil {
		return fmt.Errorf("failed to fetch remote server list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote server list returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read remote server list response: %w", err)
	}

	var remoteServers []NTPServerConfig
	if err := json.Unmarshal(body, &remoteServers); err != nil {
		if err := yaml.Unmarshal(body, &remoteServers); err != nil {
			return fmt.Errorf("failed to parse remote server list: %w", err)
		}
	}

	// Merge remote servers with existing additional servers
	c.AdditionalServers = append(c.AdditionalServers, remoteServers...)
	
	return nil
}

// GetAllEnabledServers returns all enabled NTP servers sorted by weight (descending)
func (c *Config) GetAllEnabledServers() []NTPServerConfig {
	var servers []NTPServerConfig
	
	// Add builtin servers
	for _, server := range c.BuiltinServers {
		if server.Enabled {
			servers = append(servers, server)
		}
	}
	
	// Add additional servers
	for _, server := range c.AdditionalServers {
		if server.Enabled {
			servers = append(servers, server)
		}
	}
	
	// Sort by weight (descending)
	for i := 0; i < len(servers)-1; i++ {
		for j := i + 1; j < len(servers); j++ {
			if servers[i].Weight < servers[j].Weight {
				servers[i], servers[j] = servers[j], servers[i]
			}
		}
	}
	
	return servers
}

// SaveConfigToFile saves current configuration to a file
func (c *Config) SaveConfigToFile(filename string, format string) error {
	var data []byte
	var err error
	
	switch format {
	case "json":
		data, err = json.MarshalIndent(c, "", "  ")
	case "yaml":
		data, err = yaml.Marshal(c)
	default:
		return fmt.Errorf("unsupported format: %s (use 'json' or 'yaml')", format)
	}
	
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	return os.WriteFile(filename, data, 0644)
}