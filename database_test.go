package main

import (
	"database/sql"
	"os"
	"testing"
	"time"

	dhcp "github.com/krolaw/dhcp4"
)

// setupTestDB creates a temporary test database
func setupTestDB(t *testing.T) string {
	tmpfile, err := os.CreateTemp("", "godhcp-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	return tmpfile.Name()
}

// teardownTestDB removes the test database
func teardownTestDB(t *testing.T, dbPath string) {
	CloseDatabase()
	os.Remove(dbPath)
}

func TestInitDatabase(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	// Verify database is accessible
	if db == nil {
		t.Fatal("Database connection is nil")
	}

	// Verify schema was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='dhcp_option_overrides'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 table, got %d", count)
	}
}

func TestSaveAndGetOptionOverride(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	tests := []struct {
		name    string
		ovType  string
		target  string
		options []DHCPOption
	}{
		{
			name:   "Network override with DNS",
			ovType: "network",
			target: "192.168.1.0",
			options: []DHCPOption{
				{OptionCode: 6, OptionValue: "8.8.8.8,8.8.4.4", OptionType: "ips"},
			},
		},
		{
			name:   "MAC override with lease time",
			ovType: "mac",
			target: "aa:bb:cc:dd:ee:ff",
			options: []DHCPOption{
				{OptionCode: 51, OptionValue: "7200", OptionType: "uint32"},
			},
		},
		{
			name:   "Multiple options",
			ovType: "network",
			target: "10.0.0.0",
			options: []DHCPOption{
				{OptionCode: 3, OptionValue: "10.0.0.1", OptionType: "ip"},
				{OptionCode: 6, OptionValue: "1.1.1.1,1.0.0.1", OptionType: "ips"},
				{OptionCode: 15, OptionValue: "example.com", OptionType: "string"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save option override
			err := SaveOptionOverride(tt.ovType, tt.target, tt.options)
			if err != nil {
				t.Fatalf("SaveOptionOverride failed: %v", err)
			}

			// Get option override
			override, err := GetOptionOverride(tt.ovType, tt.target)
			if err != nil {
				t.Fatalf("GetOptionOverride failed: %v", err)
			}

			if override == nil {
				t.Fatal("GetOptionOverride returned nil")
			}

			if override.Type != tt.ovType {
				t.Errorf("Expected type %s, got %s", tt.ovType, override.Type)
			}

			if override.Target != tt.target {
				t.Errorf("Expected target %s, got %s", tt.target, override.Target)
			}

			if len(override.Options) != len(tt.options) {
				t.Errorf("Expected %d options, got %d", len(tt.options), len(override.Options))
			}

			// Verify options content
			for i, opt := range tt.options {
				if override.Options[i].OptionCode != opt.OptionCode {
					t.Errorf("Option %d: expected code %d, got %d", i, opt.OptionCode, override.Options[i].OptionCode)
				}
				if override.Options[i].OptionValue != opt.OptionValue {
					t.Errorf("Option %d: expected value %s, got %s", i, opt.OptionValue, override.Options[i].OptionValue)
				}
				if override.Options[i].OptionType != opt.OptionType {
					t.Errorf("Option %d: expected type %s, got %s", i, opt.OptionType, override.Options[i].OptionType)
				}
			}
		})
	}
}

func TestUpdateOptionOverride(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	target := "192.168.1.0"
	initialOptions := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}

	// Save initial override
	err = SaveOptionOverride("network", target, initialOptions)
	if err != nil {
		t.Fatalf("Initial SaveOptionOverride failed: %v", err)
	}

	// Update with new options
	updatedOptions := []DHCPOption{
		{OptionCode: 6, OptionValue: "1.1.1.1,1.0.0.1", OptionType: "ips"},
		{OptionCode: 3, OptionValue: "192.168.1.1", OptionType: "ip"},
	}

	err = SaveOptionOverride("network", target, updatedOptions)
	if err != nil {
		t.Fatalf("Update SaveOptionOverride failed: %v", err)
	}

	// Verify update
	override, err := GetOptionOverride("network", target)
	if err != nil {
		t.Fatalf("GetOptionOverride failed: %v", err)
	}

	if len(override.Options) != 2 {
		t.Errorf("Expected 2 options after update, got %d", len(override.Options))
	}
}

func TestDeleteOptionOverride(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	target := "192.168.1.0"
	options := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}

	// Save override
	err = SaveOptionOverride("network", target, options)
	if err != nil {
		t.Fatalf("SaveOptionOverride failed: %v", err)
	}

	// Delete override
	err = DeleteOptionOverride("network", target)
	if err != nil {
		t.Fatalf("DeleteOptionOverride failed: %v", err)
	}

	// Verify deletion
	override, err := GetOptionOverride("network", target)
	if err != nil {
		t.Fatalf("GetOptionOverride failed: %v", err)
	}

	if override != nil {
		t.Error("Expected nil after deletion, got override")
	}
}

func TestDeleteNonExistentOverride(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	err = DeleteOptionOverride("network", "192.168.99.0")
	if err != sql.ErrNoRows {
		t.Errorf("Expected ErrNoRows, got %v", err)
	}
}

func TestListOptionOverrides(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	// Add multiple overrides
	overrides := []struct {
		ovType  string
		target  string
		options []DHCPOption
	}{
		{"network", "192.168.1.0", []DHCPOption{{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"}}},
		{"network", "10.0.0.0", []DHCPOption{{OptionCode: 3, OptionValue: "10.0.0.1", OptionType: "ip"}}},
		{"mac", "aa:bb:cc:dd:ee:ff", []DHCPOption{{OptionCode: 51, OptionValue: "7200", OptionType: "uint32"}}},
	}

	for _, ov := range overrides {
		err := SaveOptionOverride(ov.ovType, ov.target, ov.options)
		if err != nil {
			t.Fatalf("SaveOptionOverride failed: %v", err)
		}
	}

	// List all overrides
	allOverrides, err := ListOptionOverrides("")
	if err != nil {
		t.Fatalf("ListOptionOverrides failed: %v", err)
	}

	if len(allOverrides) != 3 {
		t.Errorf("Expected 3 overrides, got %d", len(allOverrides))
	}

	// List network overrides only
	networkOverrides, err := ListOptionOverrides("network")
	if err != nil {
		t.Fatalf("ListOptionOverrides(network) failed: %v", err)
	}

	if len(networkOverrides) != 2 {
		t.Errorf("Expected 2 network overrides, got %d", len(networkOverrides))
	}

	// List MAC overrides only
	macOverrides, err := ListOptionOverrides("mac")
	if err != nil {
		t.Fatalf("ListOptionOverrides(mac) failed: %v", err)
	}

	if len(macOverrides) != 1 {
		t.Errorf("Expected 1 MAC override, got %d", len(macOverrides))
	}
}

func TestConvertOptionToDHCP(t *testing.T) {
	tests := []struct {
		name        string
		option      DHCPOption
		expectError bool
		checkValue  func([]byte) bool
	}{
		{
			name:        "Single IP",
			option:      DHCPOption{OptionCode: 3, OptionValue: "192.168.1.1", OptionType: "ip"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return len(b) == 4 && b[0] == 192 && b[1] == 168 && b[2] == 1 && b[3] == 1
			},
		},
		{
			name:        "Multiple IPs",
			option:      DHCPOption{OptionCode: 6, OptionValue: "8.8.8.8,8.8.4.4", OptionType: "ips"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return len(b) == 8 // Two IPv4 addresses
			},
		},
		{
			name:        "String",
			option:      DHCPOption{OptionCode: 15, OptionValue: "example.com", OptionType: "string"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return string(b) == "example.com"
			},
		},
		{
			name:        "Uint32",
			option:      DHCPOption{OptionCode: 51, OptionValue: "3600", OptionType: "uint32"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return len(b) == 4
			},
		},
		{
			name:        "Uint16",
			option:      DHCPOption{OptionCode: 57, OptionValue: "1500", OptionType: "uint16"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return len(b) == 2
			},
		},
		{
			name:        "Uint8",
			option:      DHCPOption{OptionCode: 1, OptionValue: "255", OptionType: "uint8"},
			expectError: false,
			checkValue: func(b []byte) bool {
				return len(b) == 1 && b[0] == 255
			},
		},
		{
			name:        "Invalid IP",
			option:      DHCPOption{OptionCode: 3, OptionValue: "999.999.999.999", OptionType: "ip"},
			expectError: true,
		},
		{
			name:        "Invalid uint32",
			option:      DHCPOption{OptionCode: 51, OptionValue: "not-a-number", OptionType: "uint32"},
			expectError: true,
		},
		{
			name:        "Unsupported type",
			option:      DHCPOption{OptionCode: 1, OptionValue: "test", OptionType: "unknown"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, value, err := ConvertOptionToDHCP(tt.option)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if code != dhcp.OptionCode(tt.option.OptionCode) {
				t.Errorf("Expected code %d, got %d", tt.option.OptionCode, code)
			}

			if tt.checkValue != nil && !tt.checkValue(value) {
				t.Errorf("Value check failed for %s: got %v", tt.name, value)
			}
		})
	}
}

func TestApplyOptionOverrides(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	// Setup test overrides
	networkOptions := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}
	err = SaveOptionOverride("network", "192.168.1.0", networkOptions)
	if err != nil {
		t.Fatalf("SaveOptionOverride failed: %v", err)
	}

	macOptions := []DHCPOption{
		{OptionCode: 6, OptionValue: "1.1.1.1", OptionType: "ip"},
	}
	err = SaveOptionOverride("mac", "aa:bb:cc:dd:ee:ff", macOptions)
	if err != nil {
		t.Fatalf("SaveOptionOverride failed: %v", err)
	}

	// Base options
	baseOptions := dhcp.Options{
		dhcp.OptionSubnetMask: []byte{255, 255, 255, 0},
	}

	// Test 1: Apply network override only
	result1 := ApplyOptionOverrides(baseOptions, "192.168.1.0", "")
	if _, exists := result1[dhcp.OptionDomainNameServer]; !exists {
		t.Error("Expected DNS option from network override")
	}

	// Test 2: Apply both network and MAC overrides (MAC should take precedence)
	result2 := ApplyOptionOverrides(baseOptions, "192.168.1.0", "aa:bb:cc:dd:ee:ff")
	dnsValue := result2[dhcp.OptionDomainNameServer]
	if len(dnsValue) != 4 || dnsValue[0] != 1 || dnsValue[1] != 1 {
		t.Error("Expected MAC override to take precedence over network override")
	}

	// Test 3: Base options should be preserved
	if _, exists := result2[dhcp.OptionSubnetMask]; !exists {
		t.Error("Base options should be preserved")
	}
}

func TestConcurrentAccess(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	// Test concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			target := "192.168." + string(rune(i)) + ".0"
			options := []DHCPOption{
				{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
			}
			err := SaveOptionOverride("network", target, options)
			if err != nil {
				t.Errorf("Concurrent SaveOptionOverride failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test concurrent reads
	for i := 0; i < 10; i++ {
		go func(i int) {
			_, err := ListOptionOverrides("network")
			if err != nil {
				t.Errorf("Concurrent ListOptionOverrides failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestTimestamps(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)

	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	target := "192.168.1.0"
	options := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}

	// Create override
	err = SaveOptionOverride("network", target, options)
	if err != nil {
		t.Fatalf("SaveOptionOverride failed: %v", err)
	}

	override1, err := GetOptionOverride("network", target)
	if err != nil {
		t.Fatalf("GetOptionOverride failed: %v", err)
	}

	if override1.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	if override1.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Wait to ensure timestamp changes (SQLite CURRENT_TIMESTAMP has second precision)
	time.Sleep(1100 * time.Millisecond)

	updatedOptions := []DHCPOption{
		{OptionCode: 3, OptionValue: "192.168.1.1", OptionType: "ip"},
	}
	err = SaveOptionOverride("network", target, updatedOptions)
	if err != nil {
		t.Fatalf("SaveOptionOverride update failed: %v", err)
	}

	override2, err := GetOptionOverride("network", target)
	if err != nil {
		t.Fatalf("GetOptionOverride failed: %v", err)
	}

	// CreatedAt should remain the same
	if !override1.CreatedAt.Equal(override2.CreatedAt) {
		t.Error("CreatedAt should not change on update")
	}

	// UpdatedAt should be newer
	if !override2.UpdatedAt.After(override1.UpdatedAt) {
		t.Error("UpdatedAt should be newer after update")
	}
}
