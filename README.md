# High-Availability NTP Client

A robust NTP client designed for edge computing environments with poor network conditions. It integrates with a high-availability DNS client and provides comprehensive NTP server testing and time synchronization capabilities.

## Features

- **🇨🇳 Built-in China Support**: Pre-configured with Aliyun NTP servers (ntp1-7.aliyun.com)
- **🌐 High-Availability DNS**: Uses redundant DNS resolution with DoH and UDP fallbacks
- **⚡ Parallel Testing**: Tests multiple NTP servers concurrently for faster results
- **📊 Comprehensive Testing**: Detailed server availability and accuracy testing
- **⚙️ Flexible Configuration**: Support for config files and remote server lists
- **🔄 Automatic Fallback**: Falls back through multiple servers and DNS methods
- **📈 Detailed Reporting**: JSON output with test results and recommendations

## Quick Start

### Basic Usage

```bash
# Test all configured NTP servers
./ha-ntp-client test

# Synchronize system time
./ha-ntp-client sync

# Show configured servers
./ha-ntp-client
```

### Generate Configuration

```bash
# Generate YAML config file
./ha-ntp-client config generate yaml

# Generate JSON config file  
./ha-ntp-client config generate json
```

### Using Configuration Files

```bash
# Use custom configuration
./ha-ntp-client --config my-config.yaml test

# Load additional servers from remote URL
./ha-ntp-client --remote-url https://example.com/ntp-servers.json test
```

## Installation

### Prerequisites

- Go 1.23 or later
- Access to the companion `ha-dns-client` and Facebook `time` libraries

### Build from Source

```bash
cd ha-ntp-client
go mod tidy
go build -o ha-ntp-client
```

## Configuration

The client can be configured via YAML or JSON files. Here's a sample configuration:

```yaml
# High-Availability NTP Client Configuration
builtin_servers:
  - host: ntp1.aliyun.com
    port: 123
    weight: 100
    enabled: true
    location: China

additional_servers:
  - host: cn.pool.ntp.org
    port: 123
    weight: 70
    enabled: true
    location: China

# DNS Configuration
dns_timeout: 10s
use_ha_dns: true

# NTP Configuration
ntp_timeout: 5s
max_retries: 3
parallel_queries: 5
sync_threshold: 100ms

# Remote server list (optional)
remote_server_list_url: "https://example.com/ntp-servers.json"
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `builtin_servers` | Pre-configured NTP servers | Aliyun + Global servers |
| `additional_servers` | Extra servers from config | Empty |
| `remote_server_list_url` | URL to fetch server list | Empty |
| `use_ha_dns` | Enable HA DNS resolution | true |
| `dns_timeout` | DNS resolution timeout | 10s |
| `ntp_timeout` | NTP query timeout | 5s |
| `max_retries` | Retry attempts per server | 3 |
| `parallel_queries` | Concurrent server tests | 5 |
| `sync_threshold` | Max acceptable time offset | 100ms |

## Command Line Options

### Global Flags

- `--config, -c`: Configuration file path
- `--ha-dns`: Enable/disable HA DNS (default: true)
- `--parallel`: Number of parallel queries (default: 5)
- `--retries`: Maximum retry attempts (default: 3)
- `--remote-url`: Remote URL for additional servers

### Commands

#### `test`
Test all configured NTP servers for availability and accuracy.

```bash
./ha-ntp-client test [flags]
```

Flags:
- `--timeout`: Test timeout duration (default: 30s)
- `--output`: Test results output file (default: ntp_test_results.json)

#### `sync`
Synchronize system time using the best available NTP server.

```bash
./ha-ntp-client sync
```

#### `config generate`
Generate default configuration files.

```bash
./ha-ntp-client config generate [json|yaml]
```

## High-Availability DNS Integration

The client integrates with the `ha-dns-client` library to provide robust domain name resolution:

1. **DoH (DNS over HTTPS)**: Primary resolution method using multiple providers
2. **UDP DNS**: Fallback to traditional DNS servers
3. **System DNS**: Final fallback to system resolver

This ensures NTP servers can be reached even when some DNS services are unavailable.

## Built-in NTP Servers

The client comes pre-configured with reliable NTP servers:

### China (Primary)
- ntp1.aliyun.com - ntp7.aliyun.com (Weight: 90-100)

### Global (Backup)
- time.google.com (Weight: 60)
- time.cloudflare.com (Weight: 60) 
- pool.ntp.org (Weight: 50)

Servers are prioritized by weight, with higher weights tested first.

## Test Results

The test command generates detailed JSON reports:

```json
{
  "test_time": "2024-01-01T12:00:00Z",
  "total_tests": 10,
  "successful": 8,
  "failed": 2,
  "best_server": {
    "server": {"host": "ntp1.aliyun.com", "port": 123},
    "ip": "203.107.6.88",
    "offset": "5ms",
    "delay": "25ms",
    "success": true
  },
  "all_results": [],
  "summary": "Best server: ntp1.aliyun.com with offset 5ms"
}
```

## Remote Server Lists

You can configure remote URLs to fetch additional NTP servers:

```json
[
  {
    "host": "time1.example.com",
    "port": 123,
    "weight": 80,
    "enabled": true,
    "location": "Custom"
  }
]
```

The remote list supports both JSON and YAML formats.

## Error Handling

The client implements comprehensive error handling:

- **DNS Resolution Failures**: Falls back through multiple DNS methods
- **NTP Server Failures**: Retries with exponential backoff
- **Network Issues**: Tests multiple servers in parallel
- **Configuration Errors**: Provides detailed error messages

## Testing Time Synchronization

### Verify Current Time Accuracy

```bash
# Test all servers and check time offset
./ha-ntp-client test

# Quick sync check
./ha-ntp-client sync
```

### Demo: Testing with Incorrect System Time

To demonstrate the time synchronization effectiveness:

```bash
# 1. Check current time and offset
./ha-ntp-client test | grep "Best server"

# 2. Deliberately set incorrect time (requires sudo)
sudo date -s "12:30:00"
echo "System time set to: $(date)"

# 3. Test time offset (should show large offset)
./ha-ntp-client test --parallel 3

# 4. Synchronize time
./ha-ntp-client sync

# 5. Verify synchronization worked
./ha-ntp-client test | grep "Best server"
```

### Expected Results

- **Before sync**: Large time offset (potentially minutes/hours)
- **After sync**: Small offset (typically < 10ms for global servers, ~50ms for China servers)
- **Sync threshold**: Default 100ms (configurable)

### Interpreting Test Results

```json
{
  "best_server": {
    "server": {"host": "time.google.com"},
    "ip": "216.239.35.0",
    "offset": 3992587,
    "delay": 1853165,
    "success": true
  }
}
```

- **offset**: 3992587 (3.99ms offset in nanoseconds)
- **delay**: 1853165 (1.85ms network delay in nanoseconds)

- **Offset**: Time difference between local and server time
- **Delay**: Network round-trip time
- **Lower values**: Better server performance

## Use Cases

### Edge Computing
- Reliable time sync in environments with unstable internet
- Multiple fallback options for DNS and NTP
- Optimized for high-latency networks

### China-Specific Deployments
- Pre-configured with fast local NTP servers
- Handles DNS restrictions and network policies
- Backup global servers for redundancy

### Testing and Monitoring
- Comprehensive server health checking
- Performance metrics and reporting
- Automated selection of best servers

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project builds upon Facebook's NTP library and follows the Apache 2.0 License.

## Support

For issues and questions:
1. Check the test results for server-specific problems
2. Verify network connectivity and DNS resolution
3. Review configuration file syntax
4. Enable verbose logging for detailed troubleshooting