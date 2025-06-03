package main

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var (
	configFile       string
	testMode         bool
	testTimeout      time.Duration
	testResultFile   string
	useHADNS         bool
	parallelQueries  int
	maxRetries       int
	syncMode         bool
	generateConfig   string
	remoteURL        string
)

var rootCmd = &cobra.Command{
	Use:   "ha-ntp-client",
	Short: "High Availability NTP Client",
	Long: `A high-availability NTP client that uses redundant DNS resolution and multiple NTP servers
to provide reliable time synchronization in edge computing environments with poor network conditions.

Features:
- Built-in Aliyun NTP servers for China
- High-availability DNS resolution using DoH and UDP fallbacks
- Parallel NTP server testing
- Configuration file and remote URL support
- Comprehensive testing and reporting`,
	Run: func(cmd *cobra.Command, args []string) {
		if generateConfig != "" {
			generateConfigFile()
			return
		}
		
		runClient()
	},
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test NTP servers availability and accuracy",
	Long: `Test all configured NTP servers to check their availability and time accuracy.
This mode will test each server multiple times and provide detailed results.`,
	Run: func(cmd *cobra.Command, args []string) {
		testMode = true
		runClient()
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize system time using best available NTP server",
	Long: `Synchronize system time by finding the best available NTP server and adjusting
the system clock. Requires appropriate system privileges.`,
	Run: func(cmd *cobra.Command, args []string) {
		syncMode = true
		runClient()
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  "Generate or manage configuration files",
}

var generateConfigCmd = &cobra.Command{
	Use:   "generate [format]",
	Short: "Generate default configuration file",
	Long: `Generate a default configuration file in JSON or YAML format.
Format can be 'json' or 'yaml' (default: json)`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		format := "json"
		if len(args) > 0 {
			format = args[0]
		}
		
		if format != "json" && format != "yaml" {
			log.Fatalf("❌ Invalid format: %s (use 'json' or 'yaml')", format)
		}
		
		config := LoadDefaultConfig()
		filename := fmt.Sprintf("ha-ntp-config.%s", format)
		
		err := config.SaveConfigToFile(filename, format)
		if err != nil {
			log.Fatalf("❌ Failed to generate config: %v", err)
		}
		
		fmt.Printf("✅ Default configuration saved to: %s\n", filename)
	},
}

func init() {
	// Root command flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Configuration file path")
	rootCmd.PersistentFlags().BoolVar(&useHADNS, "ha-dns", true, "Use high-availability DNS client")
	rootCmd.PersistentFlags().IntVar(&parallelQueries, "parallel", 5, "Number of parallel NTP queries")
	rootCmd.PersistentFlags().IntVar(&maxRetries, "retries", 3, "Maximum retry attempts per server")
	rootCmd.PersistentFlags().StringVar(&remoteURL, "remote-url", "", "Remote URL to load additional NTP servers")
	
	// Test command flags
	testCmd.Flags().DurationVar(&testTimeout, "timeout", 30*time.Second, "Test timeout duration")
	testCmd.Flags().StringVar(&testResultFile, "output", "ntp_test_results.json", "Test results output file")
	
	// Generate config flags
	rootCmd.Flags().StringVar(&generateConfig, "generate-config", "", "Generate default config file (json|yaml)")
	
	// Add subcommands
	configCmd.AddCommand(generateConfigCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(configCmd)
}

func generateConfigFile() {
	if generateConfig != "json" && generateConfig != "yaml" {
		log.Fatalf("❌ Invalid format: %s (use 'json' or 'yaml')", generateConfig)
	}
	
	config := LoadDefaultConfig()
	filename := fmt.Sprintf("ha-ntp-config.%s", generateConfig)
	
	err := config.SaveConfigToFile(filename, generateConfig)
	if err != nil {
		log.Fatalf("❌ Failed to generate config: %v", err)
	}
	
	fmt.Printf("✅ Default configuration saved to: %s\n", filename)
}

func loadConfig() *Config {
	var config *Config
	var err error
	
	if configFile != "" {
		fmt.Printf("📄 Loading configuration from: %s\n", configFile)
		config, err = LoadConfigFromFile(configFile)
		if err != nil {
			log.Fatalf("❌ Failed to load config file: %v", err)
		}
	} else {
		fmt.Printf("📄 Using default configuration\n")
		config = LoadDefaultConfig()
	}
	
	// Override config with command line flags
	config.UseHADNS = useHADNS
	config.ParallelQueries = parallelQueries
	config.MaxRetries = maxRetries
	config.RemoteServerListURL = remoteURL
	config.TestTimeout = testTimeout
	config.TestResultFile = testResultFile
	
	// Load remote servers if configured
	if config.RemoteServerListURL != "" {
		fmt.Printf("🌐 Loading servers from remote URL: %s\n", config.RemoteServerListURL)
		err = config.LoadServersFromRemoteURL()
		if err != nil {
			log.Printf("⚠️ Failed to load remote servers: %v", err)
		} else {
			fmt.Printf("✅ Remote servers loaded successfully\n")
		}
	}
	
	return config
}

func runClient() {
	fmt.Printf("🚀 HA NTP Client starting...\n")
	
	config := loadConfig()
	client := NewHANTPClient(config)
	
	servers := config.GetAllEnabledServers()
	fmt.Printf("📊 Configured with %d enabled NTP servers\n", len(servers))
	
	if testMode {
		fmt.Printf("🧪 Running in test mode...\n")
		runTestMode(client, config)
	} else if syncMode {
		fmt.Printf("🕐 Running in sync mode...\n")
		runSyncMode(client)
	} else {
		fmt.Printf("📋 Running basic server list and status check...\n")
		runInfoMode(client, config)
	}
}

func runTestMode(client *HANTPClient, config *Config) {
	fmt.Printf("⏱️ Starting comprehensive NTP server testing (timeout: %v)...\n", config.TestTimeout)
	
	start := time.Now()
	results := client.TestAllServers()
	duration := time.Since(start)
	
	fmt.Printf("\n📊 Test Results Summary:\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("Test Duration: %v\n", duration)
	fmt.Printf("Total Servers: %d\n", results.TotalTests)
	fmt.Printf("Successful: %d\n", results.Successful)
	fmt.Printf("Failed: %d\n", results.Failed)
	fmt.Printf("Success Rate: %.1f%%\n", float64(results.Successful)/float64(results.TotalTests)*100)
	
	if results.BestServer != nil {
		fmt.Printf("\n🏆 Best Server:\n")
		fmt.Printf("Host: %s (%s)\n", results.BestServer.Server.Host, results.BestServer.IP)
		fmt.Printf("Location: %s\n", results.BestServer.Server.Location)
		fmt.Printf("Offset: %v\n", results.BestServer.Offset)
		fmt.Printf("Delay: %v\n", results.BestServer.Delay)
	}
	
	fmt.Printf("\n📝 Detailed Results:\n")
	fmt.Printf("==========================================\n")
	for i, result := range results.AllResults {
		status := "❌"
		if result.Success {
			status = "✅"
		}
		
		fmt.Printf("%s [%d] %s (%s) - %s\n", 
			status, i+1, result.Server.Host, result.Server.Location, result.IP)
		
		if result.Success {
			fmt.Printf("    Offset: %v, Delay: %v\n", result.Offset, result.Delay)
		} else {
			fmt.Printf("    Error: %s\n", result.Error)
		}
	}
	
	// Save results to file
	err := client.SaveTestResults(results, config.TestResultFile)
	if err != nil {
		log.Printf("⚠️ Failed to save test results: %v", err)
	}
	
	fmt.Printf("\n✅ Test completed successfully!\n")
}

func runSyncMode(client *HANTPClient) {
	fmt.Printf("🕐 Starting time synchronization...\n")
	
	err := client.SyncTimeFromBestServer()
	if err != nil {
		log.Fatalf("❌ Time synchronization failed: %v", err)
	}
	
	fmt.Printf("✅ Time synchronization completed!\n")
}

func runInfoMode(client *HANTPClient, config *Config) {
	servers := config.GetAllEnabledServers()
	
	fmt.Printf("\n📋 Configured NTP Servers:\n")
	fmt.Printf("==========================================\n")
	for i, server := range servers {
		status := "✅"
		if !server.Enabled {
			status = "❌"
		}
		
		fmt.Printf("%s [%d] %s:%d (weight: %d, location: %s)\n", 
			status, i+1, server.Host, server.Port, server.Weight, server.Location)
	}
	
	fmt.Printf("\n💡 Usage Tips:\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("• Run 'ha-ntp-client test' to test all servers\n")
	fmt.Printf("• Run 'ha-ntp-client sync' to synchronize time\n")
	fmt.Printf("• Use '--config file.yaml' to load custom configuration\n")
	fmt.Printf("• Use '--generate-config yaml' to create a config file\n")
	
	fmt.Printf("\n🌐 DNS Configuration:\n")
	fmt.Printf("==========================================\n")
	if config.UseHADNS {
		fmt.Printf("✅ High-Availability DNS: Enabled\n")
		fmt.Printf("   - DoH servers with fallback to UDP DNS\n")
		fmt.Printf("   - Multiple DNS providers for redundancy\n")
	} else {
		fmt.Printf("❌ High-Availability DNS: Disabled (using system DNS)\n")
	}
}

func Execute() error {
	return rootCmd.Execute()
}