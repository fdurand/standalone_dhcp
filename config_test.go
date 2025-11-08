package main

import (
	"encoding/binary"
	"net"
	"testing"

	dhcp "github.com/krolaw/dhcp4"
	"github.com/fdurand/standalone_dhcp/pool"
)

func TestAssignIP(t *testing.T) {
	// Setup handler with pool
	startIP := net.ParseIP("192.168.1.10").To4()
	endIP := net.ParseIP("192.168.1.254").To4()
	poolSize := uint64(dhcp.IPRange(startIP, endIP))
	available := pool.NewDHCPPool(poolSize, 1)

	handler := &DHCPHandler{
		start:     startIP,
		available: available,
	}

	tests := []struct {
		name          string
		ipRange       string
		expectedCount int
		expectedIPs   []string
	}{
		{
			name:          "Single IP assignment",
			ipRange:       "aa:bb:cc:dd:ee:ff:192.168.1.100",
			expectedCount: 1,
			expectedIPs:   []string{"192.168.1.100"},
		},
		{
			name:          "Multiple IP assignments",
			ipRange:       "aa:bb:cc:dd:ee:ff:192.168.1.100,11:22:33:44:55:66:192.168.1.101",
			expectedCount: 2,
			expectedIPs:   []string{"192.168.1.100", "192.168.1.101"},
		},
		{
			name:          "Empty string",
			ipRange:       "",
			expectedCount: 0,
			expectedIPs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macToIP, ipList := AssignIP(handler, tt.ipRange)

			if len(ipList) != tt.expectedCount {
				t.Errorf("Expected %d IPs, got %d", tt.expectedCount, len(ipList))
			}

			for i, expectedIP := range tt.expectedIPs {
				if i < len(ipList) {
					if ipList[i].String() != expectedIP {
						t.Errorf("Expected IP %s, got %s", expectedIP, ipList[i].String())
					}
				}
			}

			// Verify MAC to position mapping
			if tt.expectedCount > 0 {
				for mac, position := range macToIP {
					expectedIP := binary.BigEndian.Uint32(handler.start) + position
					calculatedIP := make(net.IP, 4)
					binary.BigEndian.PutUint32(calculatedIP, expectedIP)

					found := false
					for _, ip := range tt.expectedIPs {
						if calculatedIP.String() == ip {
							found = true
							break
						}
					}

					if !found {
						t.Errorf("MAC %s mapped to unexpected IP position %d", mac, position)
					}
				}
			}
		})
	}
}

func TestAssignIPPosition(t *testing.T) {
	// Setup handler with pool
	startIP := net.ParseIP("192.168.1.10").To4()
	endIP := net.ParseIP("192.168.1.254").To4()
	poolSize := uint64(dhcp.IPRange(startIP, endIP))
	available := pool.NewDHCPPool(poolSize, 1)

	handler := &DHCPHandler{
		start:     startIP,
		available: available,
	}

	ipRange := "aa:bb:cc:dd:ee:ff:192.168.1.100"
	macToIP, _ := AssignIP(handler, ipRange)

	// Calculate expected position
	expectedPos := uint32(binary.BigEndian.Uint32(net.ParseIP("192.168.1.100").To4())) -
		uint32(binary.BigEndian.Uint32(handler.start))

	if position, exists := macToIP["aa:bb:cc:dd:ee:ff"]; exists {
		if position != expectedPos {
			t.Errorf("Expected position %d, got %d", expectedPos, position)
		}
	} else {
		t.Error("MAC address not found in mapping")
	}
}

func TestIPsFromRange(t *testing.T) {
	tests := []struct {
		name          string
		ipRange       string
		expectedCount int
		expectNil     bool // Whether to expect nil IPs in result
	}{
		{
			name:          "Valid range",
			ipRange:       "192.168.1.1-192.168.1.5",
			expectedCount: 5,
			expectNil:     false,
		},
		{
			name:          "Single IP",
			ipRange:       "192.168.1.1-192.168.1.1",
			expectedCount: 1,
			expectNil:     false,
		},
		{
			name:          "Empty string",
			ipRange:       "",
			expectedCount: 1, // Function returns slice with nil IP
			expectNil:     true,
		},
		{
			name:          "Invalid format",
			ipRange:       "invalid",
			expectedCount: 1, // Function returns slice with nil IP
			expectNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, count := IPsFromRange(tt.ipRange)

			if count != tt.expectedCount {
				t.Errorf("Expected count %d, got %d", tt.expectedCount, count)
			}

			if len(ips) != tt.expectedCount && tt.expectedCount > 0 {
				t.Errorf("Expected %d IPs, got %d", tt.expectedCount, len(ips))
			}
		})
	}
}

func TestShuffleIP(t *testing.T) {
	// Create a DNS IP list
	ips := []byte{8, 8, 8, 8, 8, 8, 4, 4} // 8.8.8.8 and 8.8.4.4

	// Test with different seeds
	result1 := ShuffleIP(ips, 1)
	result2 := ShuffleIP(ips, 2)

	// Results should be 8 bytes (two IPs)
	if len(result1) != 8 {
		t.Errorf("Expected 8 bytes, got %d", len(result1))
	}

	if len(result2) != 8 {
		t.Errorf("Expected 8 bytes, got %d", len(result2))
	}

	// With different seeds, results might be different (shuffled)
	// But all original IPs should still be present
	containsIP := func(data []byte, ip []byte) bool {
		for i := 0; i+3 < len(data); i += 4 {
			match := true
			for j := 0; j < 4; j++ {
				if data[i+j] != ip[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	}

	if !containsIP(result1, []byte{8, 8, 8, 8}) {
		t.Error("8.8.8.8 not found in shuffled result")
	}

	if !containsIP(result1, []byte{8, 8, 4, 4}) {
		t.Error("8.8.4.4 not found in shuffled result")
	}
}

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected bool
	}{
		{"Valid IPv4", net.ParseIP("192.168.1.1"), true},
		{"IPv6", net.ParseIP("::1"), false},
		{"IPv4-mapped IPv6", net.ParseIP("::ffff:192.168.1.1"), true},
		{"Nil IP", nil, true}, // nil.String() returns "<nil>" with 0 colons
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv4(tt.ip)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for IP %v", tt.expected, result, tt.ip)
			}
		})
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected bool
	}{
		{"Valid IPv6", net.ParseIP("::1"), true},
		{"IPv4", net.ParseIP("192.168.1.1"), false},
		{"IPv4-mapped IPv6", net.ParseIP("::ffff:192.168.1.1"), false},
		{"Nil IP", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIPv6(tt.ip)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for IP %v", tt.expected, result, tt.ip)
			}
		})
	}
}

func TestDHCPIPRange(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected int
	}{
		{"Small range", "192.168.1.1", "192.168.1.10", 10},
		{"Single IP", "192.168.1.1", "192.168.1.1", 1},
		{"Larger range", "192.168.1.1", "192.168.1.254", 254},
		{"Class C network", "192.168.0.1", "192.168.0.254", 254},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := net.ParseIP(tt.start)
			end := net.ParseIP(tt.end)
			result := dhcp.IPRange(start, end)

			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestDHCPIPAdd(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		offset   int
		expected string
	}{
		{"Add 1", "192.168.1.10", 1, "192.168.1.11"},
		{"Add 10", "192.168.1.10", 10, "192.168.1.20"},
		{"Add 0", "192.168.1.10", 0, "192.168.1.10"},
		{"Cross subnet boundary", "192.168.1.250", 10, "192.168.2.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := net.ParseIP(tt.base)
			result := dhcp.IPAdd(base, tt.offset)

			if result.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result.String())
			}
		})
	}
}
