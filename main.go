package main

import (
	"fmt"

	"context"
	_ "expvar"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/davecgh/go-spew/spew"
	"github.com/fdurand/arp"
	cache "github.com/fdurand/go-cache"
	"github.com/go-errors/errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/inverse-inc/packetfence/go/log"
	"github.com/inverse-inc/packetfence/go/timedlock"
	dhcp "github.com/krolaw/dhcp4"
)

var DHCPConfig *Interfaces

var GlobalIpCache *cache.Cache
var GlobalMacCache *cache.Cache

var GlobalTransactionCache *cache.Cache
var GlobalTransactionLock *timedlock.RWLock

var RequestGlobalTransactionCache *cache.Cache

var VIP map[string]bool
var VIPIp map[string]net.IP

var ctx = context.Background()

var intNametoInterface map[string]*Interface

const FreeMac = "00:00:00:00:00:00"
const FakeMac = "ff:ff:ff:ff:ff:ff"

func main() {
	log.SetProcessName("godhcp")
	ctx = log.LoggerNewContext(ctx)
	arp.AutoRefresh(30 * time.Second)
	// Default http timeout
	http.DefaultClient.Timeout = 10 * time.Second

	// Initialize SQLite database for option overrides
	if err := InitDatabase("/usr/local/etc/godhcp.db"); err != nil {
		log.LoggerWContext(ctx).Error("Failed to initialize database: " + err.Error())
		os.Exit(1)
	}
	defer CloseDatabase()

	// Initialize IP cache
	GlobalIpCache = cache.New(5*time.Minute, 10*time.Minute)
	// Initialize Mac cache
	GlobalMacCache = cache.New(5*time.Minute, 10*time.Minute)

	// Initialize transaction cache
	GlobalTransactionCache = cache.New(5*time.Minute, 10*time.Minute)
	GlobalTransactionLock = timedlock.NewRWLock()
	RequestGlobalTransactionCache = cache.New(1*time.Second, 2*time.Second)

	VIP = make(map[string]bool)
	VIPIp = make(map[string]net.IP)

	// Read pfconfig
	DHCPConfig = newDHCPConfig()
	DHCPConfig.readConfig()

	// Queue value
	var (
		maxQueueSize = 100
		maxWorkers   = 100
	)

	// create job channel
	jobs := make(chan job, maxQueueSize)

	// create workers
	for i := 1; i <= maxWorkers; i++ {
		go func(i int) {
			for j := range jobs {
				doWork(i, j)
			}
		}(i)
	}

	intNametoInterface = make(map[string]*Interface)

	// Unicast listener
	for _, v := range DHCPConfig.intsNet {
		v := v
		// Create a channel for each interfaces
		intNametoInterface[v.Name] = &v
		go func() {
			v.runUnicast(ctx, jobs)
		}()

	}

	// Broadcast listener
	for _, v := range DHCPConfig.intsNet {
		v := v
		go func() {
			v.run(ctx, jobs)
		}()
	}

	// Api
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/dhcp/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleMac2Ip).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleReleaseIP).Methods("DELETE")
	router.HandleFunc("/api/v1/dhcp/ip/{ip:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleIP2Mac).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/stats", handleAllStats).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/stats/{int:.*}/{network:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleStats).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/stats/{int:.*}", handleStats).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/debug/{int:.*}/{role:(?:[^/]*)}", handleDebug).Methods("GET")
	router.HandleFunc("/api/v1/config", handleGetConfig).Methods("GET")
	router.HandleFunc("/api/v1/config", handleUpdateConfig).Methods("POST")

	// DHCP option override endpoints
	router.HandleFunc("/api/v1/dhcp/options/network/{network:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleOverrideNetworkOptions).Methods("POST")
	router.HandleFunc("/api/v1/dhcp/options/network/{network:(?:[0-9]{1,3}.){3}(?:[0-9]{1,3})}", handleRemoveNetworkOptions).Methods("DELETE")
	router.HandleFunc("/api/v1/dhcp/options/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleOverrideOptions).Methods("POST")
	router.HandleFunc("/api/v1/dhcp/options/mac/{mac:(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}}", handleRemoveOptions).Methods("DELETE")
	router.HandleFunc("/api/v1/dhcp/options", handleListOptionOverrides).Methods("GET")
	router.HandleFunc("/api/v1/dhcp/options/{type}/{target}", handleGetOptionOverride).Methods("GET")

	// Serve static web UI
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("/usr/local/share/godhcp/webui")))

	srv := &http.Server{
		Addr:        "127.0.0.1:22227",
		IdleTimeout: 5 * time.Second,
		Handler:     router,
	}

	// Systemd
	daemon.SdNotify(false, "READY=1")

	go func() {
		interval, err := daemon.SdWatchdogEnabled(false)
		if err != nil || interval == 0 {
			return
		}
		cli := &http.Client{}
		for {
			req, err := http.NewRequest("GET", "http://127.0.0.1:22227", nil)
			if err != nil {
				log.LoggerWContext(ctx).Error(err.Error())
				continue
			}
			req.Close = true
			resp, err := cli.Do(req)
			time.Sleep(100 * time.Millisecond)
			if err != nil {
				log.LoggerWContext(ctx).Error(err.Error())
				continue
			}
			resp.Body.Close()
			daemon.SdNotify(false, "WATCHDOG=1")
			time.Sleep(interval / 3)
		}
	}()
	if err := srv.ListenAndServe(); err != nil {
		log.LoggerWContext(ctx).Error("HTTP server error: " + err.Error())
	}
}

func recoverName(options dhcp.Options) {
	if r := recover(); r != nil {
		fmt.Println("recovered from ", r)
		fmt.Println(errors.Wrap(r, 2).ErrorStack())
		spew.Dump(options)
	}
}
