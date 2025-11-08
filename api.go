package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-ini/ini"
	"github.com/gorilla/mux"
	"github.com/inverse-inc/packetfence/go/api-frontend/unifiedapierrors"
	"github.com/inverse-inc/packetfence/go/log"
	dhcp "github.com/krolaw/dhcp4"
)

// Node struct
type Node struct {
	Mac    string    `json:"mac"`
	IP     string    `json:"ip"`
	EndsAt time.Time `json:"ends_at"`
}

// Stats struct
type Stats struct {
	EthernetName string            `json:"interface"`
	Net          string            `json:"network"`
	Free         int               `json:"free"`
	PercentFree  int               `json:"percentfree"`
	Used         int               `json:"used"`
	PercentUsed  int               `json:"percentused"`
	Category     string            `json:"category"`
	Options      map[string]string `json:"options"`
	Members      []Node            `json:"members"`
	Status       string            `json:"status"`
	Size         int               `json:"size"`
}

type Items struct {
	Items  []Stats `json:"items"`
	Status string  `json:"status"`
}

type ApiReq struct {
	Req          string
	NetInterface string
	NetWork      string
	Mac          string
	Role         string
}

type Options struct {
	Option dhcp.OptionCode `json:"option"`
	Value  string          `json:"value"`
	Type   string          `json:"type"`
}

type Info struct {
	Status  string `json:"status"`
	Mac     string `json:"mac,omitempty"`
	Network string `json:"network,omitempty"`
}

// encodeJSON encodes data to JSON response with error handling
func encodeJSON(res http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(res).Encode(data); err != nil {
		log.LoggerWContext(ctx).Error("Failed to encode JSON response: " + err.Error())
	}
}

// ConfigSection represents a network configuration section
type ConfigSection struct {
	Network              string `json:"network"`
	DNS                  string `json:"dns"`
	Gateway              string `json:"gateway"`
	DHCPStart            string `json:"dhcp_start"`
	DHCPEnd              string `json:"dhcp_end"`
	Netmask              string `json:"netmask"`
	DomainName           string `json:"domain_name,omitempty"`
	DHCPDefaultLeaseTime string `json:"dhcp_default_lease_time"`
	DHCPMaxLeaseTime     string `json:"dhcp_max_lease_time"`
	DHCPEnabled          string `json:"dhcpd"`
	IPReserved           string `json:"ip_reserved,omitempty"`
	IPAssigned           string `json:"ip_assigned,omitempty"`
	Algorithm            string `json:"algorithm,omitempty"`
	NextHop              string `json:"next_hop,omitempty"`
}

// ConfigResponse represents the full configuration
type ConfigResponse struct {
	Interfaces []string        `json:"interfaces"`
	Relay      []string        `json:"relay,omitempty"`
	Networks   []ConfigSection `json:"networks"`
}

func handleIP2Mac(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	if index, expiresAt, found := GlobalIpCache.GetWithExpiration(vars["ip"]); found {
		var node = &Node{Mac: index.(string), IP: vars["ip"], EndsAt: expiresAt}

		outgoingJSON, err := json.Marshal(node)

		if err != nil {
			unifiedapierrors.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprint(res, string(outgoingJSON))
		return
	}
	unifiedapierrors.Error(res, "Cannot find match for this IP address", http.StatusNotFound)
	return
}

func handleMac2Ip(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	if index, expiresAt, found := GlobalMacCache.GetWithExpiration(vars["mac"]); found {
		var node = &Node{Mac: vars["mac"], IP: index.(string), EndsAt: expiresAt}

		outgoingJSON, err := json.Marshal(node)

		if err != nil {
			unifiedapierrors.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprint(res, string(outgoingJSON))
		return
	}
	unifiedapierrors.Error(res, "Cannot find match for this MAC address", http.StatusNotFound)
	return
}

func handleAllStats(res http.ResponseWriter, req *http.Request) {
	var result Items
	cfg, err := ini.Load("/usr/local/etc/godhcp.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}

	Interfaces := cfg.Section("interfaces").Key("listen").String()

	NetInterfaces := strings.Split(Interfaces, ",")

	if len(Interfaces) == 0 {
		result.Items = append(result.Items, Stats{})
	}
	for _, i := range NetInterfaces {
		if h, ok := intNametoInterface[i]; ok {
			stat := h.handleApiReq(ApiReq{Req: "stats", NetInterface: i, NetWork: ""})
			for _, s := range stat.([]Stats) {
				result.Items = append(result.Items, s)
			}
		}
	}

	result.Status = "200"
	outgoingJSON, error := json.Marshal(result)

	if error != nil {
		unifiedapierrors.Error(res, error.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(res, string(outgoingJSON))
	return
}

func handleStats(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	if h, ok := intNametoInterface[vars["int"]]; ok {
		stat := h.handleApiReq(ApiReq{Req: "stats", NetInterface: vars["int"], NetWork: vars["network"]})

		outgoingJSON, err := json.Marshal(stat)

		if err != nil {
			unifiedapierrors.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprint(res, string(outgoingJSON))
		return
	}

	unifiedapierrors.Error(res, "Interface not found", http.StatusNotFound)
	return
}

func handleDebug(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	if h, ok := intNametoInterface[vars["int"]]; ok {
		stat := h.handleApiReq(ApiReq{Req: "debug", NetInterface: vars["int"], Role: vars["role"]})

		outgoingJSON, err := json.Marshal(stat)

		if err != nil {
			unifiedapierrors.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprint(res, string(outgoingJSON))
		return
	}
	unifiedapierrors.Error(res, "Interface not found", http.StatusNotFound)
	return
}

func handleReleaseIP(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	_ = InterfaceScopeFromMac(vars["mac"])

	var result = &Info{Mac: vars["mac"], Status: "ACK"}

	res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, result)
}

func (h *Interface) handleApiReq(Request ApiReq) interface{} {
	var stats []Stats

	// Send back stats
	if Request.Req == "stats" {
		for _, v := range h.network {
			ipv4Addr, _, erro := net.ParseCIDR(Request.NetWork + "/32")
			if erro == nil {
				if !(v.network.Contains(ipv4Addr)) {
					continue
				}
			}
			var Options map[string]string
			Options = make(map[string]string)
			Options["optionIPAddressLeaseTime"] = v.dhcpHandler.leaseDuration.String()
			for option, value := range v.dhcpHandler.options {
				optionStr := option.String()
				// Convert first character to lowercase if it's uppercase
				if len(optionStr) > 0 && optionStr[0] >= 'A' && optionStr[0] <= 'Z' {
					optionStr = strings.ToLower(optionStr[:1]) + optionStr[1:]
				}
				Options[optionStr] = Tlv.Tlvlist[int(option)].Decode.String(value)
			}

			var Members []Node
			id, _ := GlobalTransactionLock.Lock()
			members := v.dhcpHandler.hwcache.Items()
			GlobalTransactionLock.Unlock(id)
			var Status string
			var Count int
			Count = 0
			for i, item := range members {
				Count++
				result := make(net.IP, 4)
				binary.BigEndian.PutUint32(result, binary.BigEndian.Uint32(v.dhcpHandler.start.To4())+uint32(item.Object.(int)))
				Members = append(Members, Node{IP: result.String(), Mac: i, EndsAt: time.Unix(0, item.Expiration)})
			}
			_, reserved := IPsFromRange(v.dhcpHandler.ipReserved)
			if reserved != 1 {
				Count = Count + reserved
			}

			availableCount := safeUint64ToInt(v.dhcpHandler.available.FreeIPsRemaining())
			usedCount := (v.dhcpHandler.leaseRange - availableCount)
			percentfree := int((float64(availableCount) / float64(v.dhcpHandler.leaseRange)) * 100)
			percentused := int((float64(usedCount) / float64(v.dhcpHandler.leaseRange)) * 100)

			if Count == (v.dhcpHandler.leaseRange - availableCount) {
				Status = "Normal"
			} else {
				Status = "Calculated available IP " + strconv.Itoa(v.dhcpHandler.leaseRange-Count) + " is different than what we have available in the pool " + strconv.Itoa(availableCount)
			}

			stats = append(stats, Stats{EthernetName: Request.NetInterface, Net: v.network.String(), Free: availableCount, Category: v.dhcpHandler.role, Options: Options, Members: Members, Status: Status, Size: v.dhcpHandler.leaseRange, Used: usedCount, PercentFree: percentfree, PercentUsed: percentused})
		}
		return stats
	}
	// Debug
	if Request.Req == "debug" {
		for _, v := range h.network {
			if Request.Role == v.dhcpHandler.role {
				spew.Dump(v.dhcpHandler.hwcache)
				stats = append(stats, Stats{EthernetName: Request.NetInterface, Net: v.network.String(), Free: safeUint64ToInt(v.dhcpHandler.available.FreeIPsRemaining()), Category: v.dhcpHandler.role, Status: "Debug finished"})
			}
		}
		return stats
	}

	return nil
}

// handleGetConfig returns the current DHCP configuration
func handleGetConfig(res http.ResponseWriter, req *http.Request) {
	cfg, err := ini.Load("/usr/local/etc/godhcp.ini")
	if err != nil {
		unifiedapierrors.Error(res, "Failed to load configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var configResponse ConfigResponse

	// Get interfaces
	interfacesStr := cfg.Section("interfaces").Key("listen").String()
	if interfacesStr != "" {
		configResponse.Interfaces = strings.Split(interfacesStr, ",")
	}

	// Get relay
	relayStr := cfg.Section("interfaces").Key("relay").String()
	if relayStr != "" {
		configResponse.Relay = strings.Split(relayStr, ",")
	}

	// Get all network sections
	sections := cfg.SectionStrings()
	for _, section := range sections {
		if strings.HasPrefix(section, "network ") {
			networkIP := strings.TrimPrefix(section, "network ")
			sec := cfg.Section(section)

			configSection := ConfigSection{
				Network:              networkIP,
				DNS:                  sec.Key("dns").String(),
				Gateway:              sec.Key("gateway").String(),
				DHCPStart:            sec.Key("dhcp_start").String(),
				DHCPEnd:              sec.Key("dhcp_end").String(),
				Netmask:              sec.Key("netmask").String(),
				DomainName:           sec.Key("domain-name").String(),
				DHCPDefaultLeaseTime: sec.Key("dhcp_default_lease_time").String(),
				DHCPMaxLeaseTime:     sec.Key("dhcp_max_lease_time").String(),
				DHCPEnabled:          sec.Key("dhcpd").String(),
				IPReserved:           sec.Key("ip_reserved").String(),
				IPAssigned:           sec.Key("ip_assigned").String(),
				Algorithm:            sec.Key("algorithm").String(),
				NextHop:              sec.Key("next_hop").String(),
			}
			configResponse.Networks = append(configResponse.Networks, configSection)
		}
	}

	res.Header().Set("Content-Type", "application/json")
	encodeJSON(res, configResponse)
}

// handleUpdateConfig updates the DHCP configuration
func handleUpdateConfig(res http.ResponseWriter, req *http.Request) {
	var configRequest ConfigResponse

	if err := json.NewDecoder(req.Body).Decode(&configRequest); err != nil {
		unifiedapierrors.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Create new INI file
	cfg := ini.Empty()

	// Add interfaces section
	interfacesSec, err := cfg.NewSection("interfaces")
	if err != nil {
		unifiedapierrors.Error(res, "Failed to create interfaces section: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(configRequest.Interfaces) > 0 {
		interfacesSec.Key("listen").SetValue(strings.Join(configRequest.Interfaces, ","))
	}

	if len(configRequest.Relay) > 0 {
		interfacesSec.Key("relay").SetValue(strings.Join(configRequest.Relay, ","))
	}

	// Add network sections
	for _, network := range configRequest.Networks {
		sectionName := "network " + network.Network
		sec, err := cfg.NewSection(sectionName)
		if err != nil {
			unifiedapierrors.Error(res, "Failed to create network section: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if network.DNS != "" {
			sec.Key("dns").SetValue(network.DNS)
		}
		if network.Gateway != "" {
			sec.Key("gateway").SetValue(network.Gateway)
		}
		if network.DHCPStart != "" {
			sec.Key("dhcp_start").SetValue(network.DHCPStart)
		}
		if network.DHCPEnd != "" {
			sec.Key("dhcp_end").SetValue(network.DHCPEnd)
		}
		if network.Netmask != "" {
			sec.Key("netmask").SetValue(network.Netmask)
		}
		if network.DomainName != "" {
			sec.Key("domain-name").SetValue(network.DomainName)
		}
		if network.DHCPDefaultLeaseTime != "" {
			sec.Key("dhcp_default_lease_time").SetValue(network.DHCPDefaultLeaseTime)
		}
		if network.DHCPMaxLeaseTime != "" {
			sec.Key("dhcp_max_lease_time").SetValue(network.DHCPMaxLeaseTime)
		}
		if network.DHCPEnabled != "" {
			sec.Key("dhcpd").SetValue(network.DHCPEnabled)
		}
		if network.IPReserved != "" {
			sec.Key("ip_reserved").SetValue(network.IPReserved)
		}
		if network.IPAssigned != "" {
			sec.Key("ip_assigned").SetValue(network.IPAssigned)
		}
		if network.Algorithm != "" {
			sec.Key("algorithm").SetValue(network.Algorithm)
		}
		if network.NextHop != "" {
			sec.Key("next_hop").SetValue(network.NextHop)
		}
	}

	// Save to file
	if err := cfg.SaveTo("/usr/local/etc/godhcp.ini"); err != nil {
		unifiedapierrors.Error(res, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"status":  "success",
		"message": "Configuration updated successfully. Restart the service to apply changes.",
	}

	res.Header().Set("Content-Type", "application/json")
	encodeJSON(res, response)
}

// handleOverrideNetworkOptions handles POST /api/v1/dhcp/options/network/{network}
func handleOverrideNetworkOptions(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	network := vars["network"]

	var options []DHCPOption
	if err := json.NewDecoder(req.Body).Decode(&options); err != nil {
		unifiedapierrors.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate options
	for _, opt := range options {
		if opt.OptionCode < 0 || opt.OptionCode > 255 {
			unifiedapierrors.Error(res, fmt.Sprintf("Invalid option code: %d", opt.OptionCode), http.StatusBadRequest)
			return
		}
		if opt.OptionValue == "" {
			unifiedapierrors.Error(res, fmt.Sprintf("Empty value for option %d", opt.OptionCode), http.StatusBadRequest)
			return
		}
	}

	// Save to database
	if err := SaveOptionOverride("network", network, options); err != nil {
		unifiedapierrors.Error(res, "Failed to save option override: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Option overrides saved for network %s", network),
		"network": network,
		"options": options,
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, response)
}

// handleRemoveNetworkOptions handles DELETE /api/v1/dhcp/options/network/{network}
func handleRemoveNetworkOptions(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	network := vars["network"]

	if err := DeleteOptionOverride("network", network); err != nil {
		if err == sql.ErrNoRows {
			unifiedapierrors.Error(res, fmt.Sprintf("No option overrides found for network %s", network), http.StatusNotFound)
			return
		}
		unifiedapierrors.Error(res, "Failed to delete option override: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Option overrides removed for network %s", network),
		"network": network,
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, response)
}

// handleOverrideOptions handles POST /api/v1/dhcp/options/mac/{mac}
func handleOverrideOptions(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mac := vars["mac"]

	var options []DHCPOption
	if err := json.NewDecoder(req.Body).Decode(&options); err != nil {
		unifiedapierrors.Error(res, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate options
	for _, opt := range options {
		if opt.OptionCode < 0 || opt.OptionCode > 255 {
			unifiedapierrors.Error(res, fmt.Sprintf("Invalid option code: %d", opt.OptionCode), http.StatusBadRequest)
			return
		}
		if opt.OptionValue == "" {
			unifiedapierrors.Error(res, fmt.Sprintf("Empty value for option %d", opt.OptionCode), http.StatusBadRequest)
			return
		}
	}

	// Normalize MAC address
	mac = strings.ToLower(mac)

	// Save to database
	if err := SaveOptionOverride("mac", mac, options); err != nil {
		unifiedapierrors.Error(res, "Failed to save option override: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Option overrides saved for MAC %s", mac),
		"mac":     mac,
		"options": options,
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, response)
}

// handleRemoveOptions handles DELETE /api/v1/dhcp/options/mac/{mac}
func handleRemoveOptions(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	mac := vars["mac"]

	// Normalize MAC address
	mac = strings.ToLower(mac)

	if err := DeleteOptionOverride("mac", mac); err != nil {
		if err == sql.ErrNoRows {
			unifiedapierrors.Error(res, fmt.Sprintf("No option overrides found for MAC %s", mac), http.StatusNotFound)
			return
		}
		unifiedapierrors.Error(res, "Failed to delete option override: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Option overrides removed for MAC %s", mac),
		"mac":     mac,
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, response)
}

// handleListOptionOverrides handles GET /api/v1/dhcp/options
func handleListOptionOverrides(res http.ResponseWriter, req *http.Request) {
	// Get query parameter for filtering by type
	overrideType := req.URL.Query().Get("type")
	if overrideType != "" && overrideType != "network" && overrideType != "mac" {
		unifiedapierrors.Error(res, "Invalid type parameter. Must be 'network' or 'mac'", http.StatusBadRequest)
		return
	}

	overrides, err := ListOptionOverrides(overrideType)
	if err != nil {
		unifiedapierrors.Error(res, "Failed to list option overrides: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":    "success",
		"count":     len(overrides),
		"overrides": overrides,
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, response)
}

// handleGetOptionOverride handles GET /api/v1/dhcp/options/{type}/{target}
func handleGetOptionOverride(res http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	overrideType := vars["type"]
	target := vars["target"]

	if overrideType != "network" && overrideType != "mac" {
		unifiedapierrors.Error(res, "Invalid type. Must be 'network' or 'mac'", http.StatusBadRequest)
		return
	}

	override, err := GetOptionOverride(overrideType, target)
	if err != nil {
		unifiedapierrors.Error(res, "Failed to get option override: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if override == nil {
		unifiedapierrors.Error(res, fmt.Sprintf("No option override found for %s %s", overrideType, target), http.StatusNotFound)
		return
	}

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	encodeJSON(res, override)
}
