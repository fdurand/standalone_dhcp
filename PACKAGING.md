# Building Debian Package for godhcp

This document describes how to build a Debian package for the standalone DHCP server.

## Prerequisites

Install the required build dependencies:

```bash
sudo apt-get install debhelper dh-systemd golang-go dpkg-dev
```

## Building the Package

### Method 1: Using dpkg-buildpackage

```bash
# Build the package
dpkg-buildpackage -us -uc -b

# The .deb file will be created in the parent directory
ls -la ../*.deb
```

### Method 2: Using Make

```bash
# Build the package
make deb

# Clean build artifacts
make deb-clean
```

## Installing the Package

```bash
# Install the package
sudo dpkg -i ../godhcp_*.deb

# If there are dependency issues, resolve them with:
sudo apt-get install -f
```

## Package Contents

The package installs the following files:

- `/usr/local/sbin/godhcp` - Main binary
- `/usr/local/etc/godhcp.ini` - Configuration file (conffile)
- `/usr/local/share/godhcp/webui/index.html` - Web UI
- `/lib/systemd/system/godhcp.service` - Systemd service file

## Post-Installation

After installation:

1. Edit the configuration file: `/usr/local/etc/godhcp.ini`
2. Start the service: `sudo systemctl start godhcp`
3. Enable on boot: `sudo systemctl enable godhcp`
4. Access the web UI: `http://127.0.0.1:22227/`

## Removing the Package

```bash
# Remove the package (keeps config files)
sudo dpkg -r godhcp

# Purge the package (removes config files)
sudo dpkg -P godhcp
```

## Package Information

- Package name: `godhcp`
- Section: net
- Priority: optional
- Architecture: any (built for current architecture)

## Maintainer Scripts

The package includes the following maintainer scripts:

- `postinst` - Sets up permissions and enables the service
- `prerm` - Stops the service before removal
- `postrm` - Cleans up on purge

## Versioning

The version is specified in `debian/changelog`. To create a new version:

```bash
dch -v 1.2.0 "Description of changes"
```

## Build Artifacts

The build process creates temporary files in:

- `debian/godhcp/` - Staging directory
- `debian/.debhelper/` - Debhelper temporary files
- `.cache/` - Go build cache

These are cleaned automatically by `make clean` or `dh_clean`.
