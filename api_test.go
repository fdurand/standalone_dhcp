package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
)

// setupTestAPI creates a test router and database
func setupTestAPI(t *testing.T) (*mux.Router, string) {
	dbPath := setupTestDB(t)
	err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/dhcp/options/network/{network:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleOverrideNetworkOptions).Methods("POST")
	router.HandleFunc("/api/v1/dhcp/options/network/{network:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleRemoveNetworkOptions).Methods("DELETE")
	router.HandleFunc("/api/v1/dhcp/options/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleOverrideOptions).Methods("POST")
	router.HandleFunc("/api/v1/dhcp/options/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleRemoveOptions).Methods("DELETE")
	router.HandleFunc("/api/v1/dhcp/options", handleListOptionOverrides).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/options/{type}/{target}", handleGetOptionOverride).Methods("GET")

	return router, dbPath
}

func TestHandleOverrideNetworkOptions(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	tests := []struct {
		name           string
		network        string
		payload        []DHCPOption
		expectedStatus int
	}{
		{
			name:    "Valid network override",
			network: "192.168.1.0",
			payload: []DHCPOption{
				{OptionCode: 6, OptionValue: "8.8.8.8,8.8.4.4", OptionType: "ips"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Multiple options",
			network: "10.0.0.0",
			payload: []DHCPOption{
				{OptionCode: 3, OptionValue: "10.0.0.1", OptionType: "ip"},
				{OptionCode: 6, OptionValue: "1.1.1.1", OptionType: "ip"},
				{OptionCode: 15, OptionValue: "example.com", OptionType: "string"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Invalid option code",
			network: "192.168.1.0",
			payload: []DHCPOption{
				{OptionCode: 256, OptionValue: "test", OptionType: "string"},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "Empty value",
			network: "192.168.1.0",
			payload: []DHCPOption{
				{OptionCode: 6, OptionValue: "", OptionType: "ip"},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/api/v1/dhcp/options/network/"+tt.network, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				if err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if response["status"] != "success" {
					t.Errorf("Expected success status, got %v", response["status"])
				}
			}
		})
	}
}

func TestHandleOverrideOptions(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	tests := []struct {
		name           string
		mac            string
		payload        []DHCPOption
		expectedStatus int
	}{
		{
			name: "Valid MAC override",
			mac:  "aa:bb:cc:dd:ee:ff",
			payload: []DHCPOption{
				{OptionCode: 51, OptionValue: "7200", OptionType: "uint32"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Case insensitive MAC",
			mac:  "AA:BB:CC:DD:EE:FF",
			payload: []DHCPOption{
				{OptionCode: 51, OptionValue: "3600", OptionType: "uint32"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid JSON",
			mac:            "aa:bb:cc:dd:ee:ff",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.payload != nil {
				body, _ = json.Marshal(tt.payload)
			} else {
				body = []byte("invalid json")
			}

			req := httptest.NewRequest("POST", "/api/v1/dhcp/options/mac/"+tt.mac, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleRemoveNetworkOptions(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	network := "192.168.1.0"
	options := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}

	// Create override first
	body, _ := json.Marshal(options)
	req := httptest.NewRequest("POST", "/api/v1/dhcp/options/network/"+network, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to create override: %d", w.Code)
	}

	// Now delete it
	req = httptest.NewRequest("DELETE", "/api/v1/dhcp/options/network/"+network, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify it's deleted
	req = httptest.NewRequest("DELETE", "/api/v1/dhcp/options/network/"+network, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent override, got %d", w.Code)
	}
}

func TestHandleRemoveOptions(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	mac := "aa:bb:cc:dd:ee:ff"
	options := []DHCPOption{
		{OptionCode: 51, OptionValue: "7200", OptionType: "uint32"},
	}

	// Create override first
	body, _ := json.Marshal(options)
	req := httptest.NewRequest("POST", "/api/v1/dhcp/options/mac/"+mac, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to create override: %d", w.Code)
	}

	// Now delete it
	req = httptest.NewRequest("DELETE", "/api/v1/dhcp/options/mac/"+mac, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleListOptionOverrides(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	// Create some test overrides
	overrides := []struct {
		url     string
		options []DHCPOption
	}{
		{
			"/api/v1/dhcp/options/network/192.168.1.0",
			[]DHCPOption{{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"}},
		},
		{
			"/api/v1/dhcp/options/network/10.0.0.0",
			[]DHCPOption{{OptionCode: 3, OptionValue: "10.0.0.1", OptionType: "ip"}},
		},
		{
			"/api/v1/dhcp/options/mac/aa:bb:cc:dd:ee:ff",
			[]DHCPOption{{OptionCode: 51, OptionValue: "7200", OptionType: "uint32"}},
		},
	}

	for _, ov := range overrides {
		body, _ := json.Marshal(ov.options)
		req := httptest.NewRequest("POST", ov.url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Failed to create override: %d", w.Code)
		}
	}

	tests := []struct {
		name          string
		queryParam    string
		expectedCount int
	}{
		{"List all", "", 3},
		{"List networks only", "?type=network", 2},
		{"List MACs only", "?type=mac", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/dhcp/options"+tt.queryParam, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			count := int(response["count"].(float64))
			if count != tt.expectedCount {
				t.Errorf("Expected %d overrides, got %d", tt.expectedCount, count)
			}
		})
	}
}

func TestHandleGetOptionOverride(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	// Create test override
	network := "192.168.1.0"
	options := []DHCPOption{
		{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
	}

	body, _ := json.Marshal(options)
	req := httptest.NewRequest("POST", "/api/v1/dhcp/options/network/"+network, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to create override: %d", w.Code)
	}

	// Get the override
	req = httptest.NewRequest("GET", "/api/v1/dhcp/options/network/"+network, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var override OptionOverride
	err := json.Unmarshal(w.Body.Bytes(), &override)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if override.Type != "network" {
		t.Errorf("Expected type 'network', got '%s'", override.Type)
	}

	if override.Target != network {
		t.Errorf("Expected target '%s', got '%s'", network, override.Target)
	}

	if len(override.Options) != 1 {
		t.Errorf("Expected 1 option, got %d", len(override.Options))
	}

	// Test non-existent override
	req = httptest.NewRequest("GET", "/api/v1/dhcp/options/network/10.99.99.0", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestInvalidTypeParameter(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	req := httptest.NewRequest("GET", "/api/v1/dhcp/options?type=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleGetOptionOverrideInvalidType(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	req := httptest.NewRequest("GET", "/api/v1/dhcp/options/invalid/192.168.1.0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestConcurrentAPIRequests(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	// Simulate concurrent POST requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			options := []DHCPOption{
				{OptionCode: 6, OptionValue: "8.8.8.8", OptionType: "ip"},
			}
			body, _ := json.Marshal(options)

			network := "192.168." + strconv.Itoa(i) + ".0"
			req := httptest.NewRequest("POST", "/api/v1/dhcp/options/network/"+network, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent POST failed: %d", w.Code)
			}
			done <- true
		}(i)
	}

	// Wait for all requests
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all were created
	req := httptest.NewRequest("GET", "/api/v1/dhcp/options?type=network", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	count := int(response["count"].(float64))

	if count < 10 {
		t.Errorf("Expected at least 10 overrides, got %d", count)
	}
}
