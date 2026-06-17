package main

import (
	"bytes"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	cache "github.com/fdurand/go-cache"
	"github.com/gorilla/mux"
	dhcp "github.com/krolaw/dhcp4"
)

// newTestPacket builds a minimal DHCP packet with the given client MAC.
func newTestPacket(t *testing.T, mac string) dhcp.Packet {
	t.Helper()
	hw, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatalf("bad mac %q: %v", mac, err)
	}
	p := dhcp.NewPacket(dhcp.BootRequest)
	p.SetCHAddr(hw)
	return p
}

func sortedBytes(b []byte) []byte {
	out := append([]byte(nil), b...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TestBuildReplyOptions guards the OFFER/ACK consistency fix: the shared helper
// must preserve the base option set (DNS/router only reordered) and apply
// configured overrides.
func TestBuildReplyOptions(t *testing.T) {
	dbPath := setupTestDB(t)
	defer teardownTestDB(t, dbPath)
	if err := InitDatabase(dbPath); err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}

	const mac = "aa:bb:cc:dd:ee:ff"
	p := newTestPacket(t, mac)

	base := dhcp.Options{
		dhcp.OptionSubnetMask:       []byte{255, 255, 255, 0},
		dhcp.OptionRouter:           append(net.IPv4(10, 0, 0, 1).To4(), net.IPv4(10, 0, 0, 2).To4()...),
		dhcp.OptionDomainNameServer: append(net.IPv4(8, 8, 8, 8).To4(), net.IPv4(8, 8, 4, 4).To4()...),
	}

	// No overrides: every base option survives; subnet mask is untouched and
	// the DNS/router payloads carry the same bytes (only possibly reordered).
	got := buildReplyOptions(base, p, "192.168.50.0", mac)
	if !bytes.Equal(got[dhcp.OptionSubnetMask], base[dhcp.OptionSubnetMask]) {
		t.Errorf("subnet mask changed: got %v", got[dhcp.OptionSubnetMask])
	}
	if !bytes.Equal(sortedBytes(got[dhcp.OptionDomainNameServer]), sortedBytes(base[dhcp.OptionDomainNameServer])) {
		t.Errorf("DNS option content changed: got %v", got[dhcp.OptionDomainNameServer])
	}

	// MAC-level override for the router option must win.
	if err := SaveOptionOverride("mac", mac, []DHCPOption{
		{OptionCode: int(dhcp.OptionRouter), OptionValue: "172.16.0.1", OptionType: "ip"},
	}); err != nil {
		t.Fatalf("SaveOptionOverride failed: %v", err)
	}
	got = buildReplyOptions(base, p, "192.168.50.0", mac)
	if want := net.IPv4(172, 16, 0, 1).To4(); !bytes.Equal(got[dhcp.OptionRouter], want) {
		t.Errorf("override not applied: got %v want %v", got[dhcp.OptionRouter], []byte(want))
	}
}

func TestSetOptionServerIdentifier(t *testing.T) {
	handler := net.IPv4(192, 168, 1, 1).To4()

	tests := []struct {
		name string
		srv  net.IP
		want net.IP
	}{
		{"equal to handler", net.IPv4(192, 168, 1, 1), handler},
		{"zero address", net.IPv4zero, handler},
		{"broadcast address", net.IPv4bcast, handler},
		{"distinct address", net.IPv4(10, 0, 0, 5), net.IPv4(10, 0, 0, 5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setOptionServerIdentifier(tt.srv, handler)
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafeConversions(t *testing.T) {
	if got := safeUint64ToInt(uint64(math.MaxUint64)); got != math.MaxInt {
		t.Errorf("safeUint64ToInt overflow: got %d want %d", got, math.MaxInt)
	}
	if got := safeUint64ToInt(42); got != 42 {
		t.Errorf("safeUint64ToInt(42) = %d", got)
	}
	if got := safeIntToUint64(-1); got != 0 {
		t.Errorf("safeIntToUint64(-1) = %d, want 0", got)
	}
	if got := safeIntToUint64(42); got != 42 {
		t.Errorf("safeIntToUint64(42) = %d", got)
	}
	if got := safeUint32ToUint64(math.MaxUint32); got != uint64(math.MaxUint32) {
		t.Errorf("safeUint32ToUint64 = %d", got)
	}
}

// TestHandleGetOptionOverrideUppercaseMAC guards the MAC normalization fix: a
// GET with an uppercase MAC must resolve an override stored lowercased.
func TestHandleGetOptionOverrideUppercaseMAC(t *testing.T) {
	router, dbPath := setupTestAPI(t)
	defer teardownTestDB(t, dbPath)

	payload, _ := json.Marshal([]DHCPOption{
		{OptionCode: 51, OptionValue: "3600", OptionType: "uint32"},
	})
	post := httptest.NewRequest("POST", "/api/v1/dhcp/options/mac/aa:bb:cc:dd:ee:ff", bytes.NewBuffer(payload))
	post.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), post)

	get := httptest.NewRequest("GET", "/api/v1/dhcp/options/mac/AA:BB:CC:DD:EE:FF", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, get)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for uppercase MAC lookup, got %d. Body: %s", w.Code, w.Body.String())
	}
}

// TestHandleReleaseIPNotFound guards the fix that returns 404 when the MAC has
// no active lease instead of always claiming an ACK.
func TestHandleReleaseIPNotFound(t *testing.T) {
	prevCache, prevConfig := GlobalMacCache, DHCPConfig
	defer func() { GlobalMacCache, DHCPConfig = prevCache, prevConfig }()

	GlobalMacCache = cache.New(5*time.Minute, 10*time.Minute)
	DHCPConfig = newDHCPConfig()

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/dhcp/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleReleaseIP).Methods("DELETE")

	req := httptest.NewRequest("DELETE", "/api/v1/dhcp/mac/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no lease exists, got %d. Body: %s", w.Code, w.Body.String())
	}
}
