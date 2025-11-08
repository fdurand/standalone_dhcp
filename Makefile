# Makefile for godhcp

BINARY_NAME=godhcp
INSTALL_DIR=/usr/local/sbin
CONFIG_DIR=/usr/local/etc
WEBUI_DIR=/usr/local/share/godhcp/webui
SYSTEMD_DIR=/lib/systemd/system

.PHONY: all build clean install uninstall deb

all: build

build:
	go build -v -o $(BINARY_NAME)

clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -rf .cache
	rm -rf debian/godhcp
	rm -f debian/*.log
	rm -f debian/files
	rm -rf debian/.debhelper

install: build
	install -d $(INSTALL_DIR)
	install -d $(CONFIG_DIR)
	install -d $(WEBUI_DIR)
	install -m 0755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	install -m 0644 godhcp.ini $(CONFIG_DIR)/godhcp.ini
	install -m 0644 webui/index.html $(WEBUI_DIR)/index.html
	install -m 0644 webui/options.html $(WEBUI_DIR)/options.html
	install -m 0644 godhcp.service $(SYSTEMD_DIR)/godhcp.service
	systemctl daemon-reload

uninstall:
	systemctl stop godhcp || true
	systemctl disable godhcp || true
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	rm -f $(CONFIG_DIR)/godhcp.ini
	rm -rf /usr/local/share/godhcp
	rm -f $(SYSTEMD_DIR)/godhcp.service
	systemctl daemon-reload

deb:
	dpkg-buildpackage -us -uc -b

deb-clean:
	dh_clean
