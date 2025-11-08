# DHCP Server Web UI

This directory contains the web-based configuration interface for the standalone DHCP server.

## Installation

To deploy the web UI, copy the contents of this directory to `/usr/local/share/godhcp/webui/`:

```bash
sudo mkdir -p /usr/local/share/godhcp/webui
sudo cp webui/index.html /usr/local/share/godhcp/webui/
```

## Accessing the Web UI

Once the DHCP server is running, access the web UI at:

```
http://127.0.0.1:22227/
```

**Note:** The web interface is only accessible from localhost (127.0.0.1) for security reasons.

## Features

- View current DHCP server configuration
- Configure network interfaces (listen and relay)
- Manage multiple network configurations
- Add, edit, and remove DHCP networks
- Configure DHCP options:
  - IP ranges (start/end)
  - Gateway and DNS servers
  - Lease times
  - Domain name
  - Static IP assignments
  - IP reservations
  - Pool allocation algorithm

## API Endpoints

The web UI uses the following REST API endpoints:

- `GET /api/v1/config` - Retrieve current configuration
- `POST /api/v1/config` - Update configuration

## Configuration File

Changes made through the web UI are saved to:

```
/usr/local/etc/godhcp.ini
```

**Important:** After saving configuration changes, restart the DHCP service for changes to take effect:

```bash
sudo systemctl restart godhcp
```
