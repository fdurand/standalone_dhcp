# Standalone DHCP Server

A high-performance, multi-interface DHCP server written in Go with REST API support, intelligent caching, and systemd integration.

## 🚀 Features

- **Multi-interface Support**: Listen on multiple network interfaces simultaneously
- **DHCP Relay**: Forward DHCP requests to relay servers for complex network topologies
- **HTTP REST API**: Manage DHCP assignments via HTTP endpoints on port 22227
- **Intelligent Caching**: Multi-level IP/MAC address caching with TTL for optimal performance
- **Worker Pool Architecture**: 100 concurrent workers processing DHCP requests
- **Systemd Integration**: Native systemd service support with watchdog functionality
- **Real-time Statistics**: Monitor DHCP pool usage and active assignments
- **Layer 2 Support**: Handle both unicast and broadcast DHCP traffic
- **Static Reservations**: Support for MAC-to-IP static assignments

## 📋 Requirements

- Go 1.15 or later
- Root privileges (required for binding to DHCP port 67)
- Linux system with network interfaces
- Systemd (optional, for service management)

## ⚡ Quick Start

### 1. Build the Server

```bash
# Clone and build
git clone https://github.com/fdurand/standalone_dhcp.git
cd standalone_dhcp

# Build optimized binary
go build -ldflags "-s -w" -o godhcp .
```

### 2. Configure Networks

Create or edit `godhcp.ini`:

```ini
[interfaces]
# Interfaces that act as DHCP server
listen=eth1

# Interface:Relay IP - forward DHCP requests to relay address
relay=eth1.2:172.20.0.1,eth1.3:172.21.0.1

[network 192.168.1.0]
dns=8.8.8.8,8.8.4.4
gateway=192.168.1.1
dhcp_start=192.168.1.10
dhcp_end=192.168.1.254
domain-name=example.org
dhcp_max_lease_time=3600
dhcp_default_lease_time=1800
dhcpd=enabled
netmask=255.255.255.0

# Static IP assignments (MAC:IP pairs)
ip_assigned=b8:27:eb:e8:40:43:192.168.1.251,d8:3a:dd:27:67:2c:192.168.1.252
```

### 3. Run the Server

```bash
# Run directly (requires root privileges)
sudo ./godhcp

# Or install as systemd service
sudo cp godhcp.service /etc/systemd/system/
sudo systemctl enable godhcp
sudo systemctl start godhcp
```

## 🛠️ Development

### VS Code Tasks

The project includes pre-configured VS Code tasks accessible via `Ctrl+Shift+P` → "Tasks: Run Task":

- **Build DHCP Server**: Create optimized production binary
- **Build DHCP Server (Debug)**: Create debug binary with symbols
- **Test DHCP Server**: Run all tests
- **Clean Build Artifacts**: Remove build files
- **Check Code Quality**: Run `go vet` and `go fmt`

### Manual Build Commands

```bash
# Production build (optimized, smaller binary)
go build -ldflags "-s -w" -o godhcp .

# Debug build (with symbols)
go build -o godhcp-debug .

# Run tests
go test ./...

# Code quality checks
go vet ./... && go fmt ./...

# Clean build artifacts
rm -f godhcp godhcp-debug standalone_dhcp
```

## 📖 Configuration Reference

### Configuration Parameters

#### `[interfaces]` Section
- **`listen`**: Comma-separated list of interfaces to listen for DHCP requests
- **`relay`**: Interface-to-relay mappings in format `interface:relay_ip`

#### `[network X.X.X.X]` Section
- **`dns`**: DNS servers (comma-separated)
- **`gateway`**: Default gateway for the network
- **`dhcp_start`**: Start of IP range to distribute
- **`dhcp_end`**: End of IP range to distribute
- **`netmask`**: Subnet mask for the network
- **`domain-name`**: Domain name provided to DHCP clients
- **`dhcp_default_lease_time`**: Default lease time in seconds
- **`dhcp_max_lease_time`**: Maximum lease time in seconds
- **`dhcpd`**: Enable/disable DHCP for this network (`enabled`/`disabled`)
- **`ip_assigned`**: Static MAC-to-IP assignments (format: `mac:ip,mac:ip`)

## 🔌 REST API

The server provides a comprehensive REST API on `127.0.0.1:22227` for DHCP management and monitoring.

### Query IP by MAC Address

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/mac/10:1f:74:b2:f6:a5
```

**Response:**
```json
{
    "result": {
        "ip": "192.168.1.100",
        "mac": "10:1f:74:b2:f6:a5"
    }
}
```

### Query MAC by IP Address

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/ip/192.168.1.100
```

**Response:**
```json
{
    "result": {
        "ip": "192.168.1.100",
        "mac": "10:1f:74:b2:f6:a5"
    }
}
```

### Release IP Assignment

Remove a MAC-to-IP binding from the cache:

```bash
curl -X DELETE http://127.0.0.1:22227/api/v1/dhcp/mac/10:1f:74:b2:f6:a5
```

### Statistics and Monitoring

#### All Interfaces Statistics

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/stats
```

#### Specific Interface Statistics

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/stats/eth1
```

#### Interface + Network Statistics

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/stats/eth1/192.168.1.0
```

**Example Response:**
```json
[
    {
        "category": "registration",
        "free": 253,
        "interface": "eth1",
        "members": [
            {
                "mac": "10:1f:74:b2:f6:a5",
                "ip": "192.168.1.100"
            }
        ],
        "network": "192.168.1.0/24",
        "options": {
            "optionDomainName": "example.org",
            "optionDomainNameServer": "8.8.8.8",
            "optionIPAddressLeaseTime": "1800",
            "optionRouter": "192.168.1.1",
            "optionSubnetMask": "255.255.255.0"
        }
    }
]
```

### Debug Information

Get detailed debugging information for interface and role:

```bash
curl http://127.0.0.1:22227/api/v1/dhcp/debug/eth1/registration
```

## 🏗️ Architecture

### Core Components

- **`main.go`**: Application entry point, HTTP server, and initialization
- **`config.go`**: INI-based configuration management and DHCP handlers
- **`interface.go`**: Network interface handling and DHCP protocol logic
- **`api.go`**: REST API endpoint handlers
- **`server.go`**: Core DHCP server functionality
- **`dictionary.go`**: DHCP option parsing and TLV handling
- **`pool/`**: IP address pool management and allocation
- **`workers_pool.go`**: Concurrent request processing

### Performance Features

**Multi-Level Caching:**
- **IP Cache**: Maps IPs to lease information (5min TTL)
- **MAC Cache**: Maps MAC addresses to assignments (5min TTL)
- **Transaction Cache**: Tracks DHCP transactions (5min TTL)
- **Request Cache**: Prevents duplicate processing (1sec TTL)

**Concurrency Model:**
- **100 Worker Goroutines**: Process DHCP requests concurrently
- **100-Request Buffer**: Handle traffic spikes without blocking
- **Per-Interface Listeners**: Separate goroutines for each network interface
- **Transaction Locking**: Prevent race conditions in IP assignments

## 🔧 Troubleshooting

### Common Issues

**Permission Denied:**
```bash
# DHCP requires root privileges
sudo ./godhcp
```

**Port 67 Already in Use:**
```bash
# Stop conflicting DHCP services
sudo systemctl stop isc-dhcp-server dhcpcd
sudo pkill dnsmasq
```

**Interface Not Found:**
```bash
# Verify interface names
ip link show
# Update godhcp.ini with correct interface names
```

**No DHCP Responses:**
```bash
# Check firewall rules
sudo iptables -I INPUT -p udp --dport 67 -j ACCEPT
sudo iptables -I OUTPUT -p udp --sport 67 -j ACCEPT
```

### Logging and Monitoring

- **Systemd Logs**: `journalctl -u godhcp -f`
- **API Health**: `curl http://127.0.0.1:22227/api/v1/dhcp/stats`
- **Process Status**: `ps aux | grep godhcp`

## 📁 Project Structure

```
├── main.go              # Application entry point
├── config.go            # Configuration management
├── interface.go         # DHCP protocol handling
├── api.go              # REST API endpoints
├── server.go           # Core DHCP server logic
├── serverif.go         # Server interface utilities
├── dictionary.go       # DHCP option parsing
├── utils.go            # Utility functions
├── rawClient.go        # Raw socket client
├── workers_pool.go     # Worker pool management
├── pool/               # IP address pool management
│   ├── pool.go
│   └── pool_test.go
├── godhcp.ini          # Default configuration
├── godhcp.service      # Systemd service file
├── .vscode/            # VS Code tasks and settings
│   └── tasks.json
├── .gitignore          # Git ignore patterns
└── README.md           # This documentation
```

## 📄 License

This project is licensed under the terms specified in the [LICENSE](LICENSE) file.

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch: `git checkout -b feature/amazing-feature`
3. Commit your changes: `git commit -m 'Add amazing feature'`
4. Push to the branch: `git push origin feature/amazing-feature`
5. Open a Pull Request

## 🙏 Acknowledgments

- Built with [krolaw/dhcp4](https://github.com/krolaw/dhcp4) DHCP library
- Uses [PacketFence](https://github.com/inverse-inc/packetfence) logging framework
- Caching provided by [go-cache](https://github.com/fdurand/go-cache)
