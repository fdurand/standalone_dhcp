package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	cache "github.com/fdurand/go-cache"
	"github.com/fdurand/standalone_dhcp/pool"
	"github.com/go-ini/ini"
	"github.com/inverse-inc/packetfence/go/log"
	dhcp "github.com/krolaw/dhcp4"
)

// DHCPHandler struct
type DHCPHandler struct {
	ip            net.IP // Server IP to use
	vip           net.IP
	options       dhcp.Options  // Options to send to DHCP Clients
	start         net.IP        // Start of IP range to distribute
	leaseRange    int           // Number of IPs to distribute (starting from start)
	leaseDuration time.Duration // Lease period
	hwcache       *cache.Cache
	xid           *cache.Cache
	available     *pool.DHCPPool // DHCPPool keeps track of the available IPs in the pool
	layer2        bool
	role          string
	ipReserved    string
	ipAssigned    map[string]uint32
}

type Interfaces struct {
	intsNet []Interface
}

type Interface struct {
	Name          string
	intNet        *net.Interface
	network       []Network
	layer2        []*net.IPNet
	Ipv4          net.IP
	Ipv6          net.IP
	InterfaceType string
	relayIP       net.IP
	listenPort    int
}

type Network struct {
	network     net.IPNet
	dhcpHandler *DHCPHandler
}

const bootp_client = 68
const bootp_server = 67

func newDHCPConfig() *Interfaces {
	var p Interfaces
	return &p
}

func (d *Interfaces) readConfig() {

	cfg, err := ini.Load("/usr/local/etc/godhcp.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}

	Interfaces := cfg.Section("interfaces").Key("listen").String()
	NetInterfaces := strings.Split(Interfaces, ",")

	networks := cfg.SectionStrings()
	networkKey, _ := regexp.Compile("^network (?P<Net>.*)$")

	for _, v := range NetInterfaces {
		eth, err := net.InterfaceByName(v)
		if err != nil {
			log.LoggerWContext(ctx).Error("Cannot find interface " + v + " on the system due to an error: " + err.Error())
			continue
		} else if eth == nil {
			log.LoggerWContext(ctx).Error("Cannot find interface " + v + " on the system")
			continue
		}

		var ethIf Interface

		ethIf.intNet = eth
		ethIf.Name = eth.Name
		ethIf.InterfaceType = "server"
		ethIf.listenPort = bootp_server

		adresses, _ := eth.Addrs()
		for _, adresse := range adresses {
			var NetIP *net.IPNet
			var IP net.IP
			IP, NetIP, _ = net.ParseCIDR(adresse.String())

			a, b := NetIP.Mask.Size()
			if a == b {
				continue
			}
			if IsIPv6(IP) {
				ethIf.Ipv6 = IP
				continue
			}
			if IsIPv4(IP) {
				ethIf.Ipv4 = IP
			}

			ethIf.layer2 = append(ethIf.layer2, NetIP)

			for _, key := range networks {
				if networkKey.MatchString(key) {
					sec := cfg.Section(key)
					netWork := networkKey.FindStringSubmatch(key)
					if sec.Key("dhcpd").String() == "disabled" {
						continue
					}
					if (NetIP.Contains(net.ParseIP(sec.Key("dhcp_start").String())) && NetIP.Contains(net.ParseIP(sec.Key("dhcp_end").String()))) || NetIP.Contains(net.ParseIP(sec.Key("next_hop").String())) {
						if int(binary.BigEndian.Uint32(net.ParseIP(sec.Key("dhcp_start").String()).To4())) > int(binary.BigEndian.Uint32(net.ParseIP(sec.Key("dhcp_end").String()).To4())) {
							log.LoggerWContext(ctx).Error("Wrong configuration, check your network " + key)
							continue
						}

						var DHCPNet Network
						var DHCPScope *DHCPHandler
						DHCPScope = &DHCPHandler{}
						DHCPNet.network.IP = net.ParseIP(netWork[1])
						DHCPNet.network.Mask = net.IPMask(net.ParseIP(sec.Key("netmask").String()))
						DHCPScope.ip = IP.To4()

						DHCPScope.role = "none"
						DHCPScope.start = net.ParseIP(sec.Key("dhcp_start").String())
						seconds, _ := strconv.Atoi(sec.Key("dhcp_default_lease_time").String())
						DHCPScope.leaseDuration = time.Duration(seconds) * time.Second
						DHCPScope.leaseRange = dhcp.IPRange(net.ParseIP(sec.Key("dhcp_start").String()), net.ParseIP(sec.Key("dhcp_end").String()))
						algorithm, _ := strconv.Atoi(sec.Key("algorithm").String())
						// Initialize dhcp pool
						available := pool.NewDHCPPool(uint64(dhcp.IPRange(net.ParseIP(sec.Key("dhcp_start").String()), net.ParseIP(sec.Key("dhcp_end").String()))), algorithm)
						DHCPScope.available = available

						// Initialize hardware cache
						hwcache := cache.New(time.Duration(seconds)*time.Second, 10*time.Second)

						hwcache.OnEvicted(func(nic string, pool interface{}) {
							go func() {
								// Always wait 30 seconds before releasing the IP again
								time.Sleep(30 * time.Second)
								log.LoggerWContext(ctx).Info(nic + " " + dhcp.IPAdd(DHCPScope.start, pool.(int)).String() + " Added back in the pool " + DHCPScope.role + " on index " + strconv.Itoa(pool.(int)))
								DHCPScope.available.FreeIPIndex(uint64(pool.(int)))
							}()
						})

						DHCPScope.hwcache = hwcache

						xid := cache.New(time.Duration(4)*time.Second, 2*time.Second)

						DHCPScope.xid = xid

						ExcludeIP(DHCPScope, sec.Key("ip_reserved").String())
						DHCPScope.ipReserved = sec.Key("ip_reserved").String()
						DHCPScope.layer2 = true
						var options = make(map[dhcp.OptionCode][]byte)

						options[dhcp.OptionSubnetMask] = []byte(net.ParseIP(sec.Key("netmask").String()).To4())
						options[dhcp.OptionDomainNameServer] = ShuffleDNS(sec)
						options[dhcp.OptionRouter] = ShuffleGateway(sec)
						options[dhcp.OptionDomainName] = []byte(sec.Key("domain-name").String())
						DHCPScope.options = options
						DHCPNet.dhcpHandler = DHCPScope

						ethIf.network = append(ethIf.network, DHCPNet)
					}
				}
			}
			d.intsNet = append(d.intsNet, ethIf)
		}
	}
	Interfaces = cfg.Section("interfaces").Key("relay").String()
	result := strings.Split(Interfaces, ",")

	for i := range result {
		if result[i] == "" {
			continue
		}
		var ethIf Interface
		ethIf.InterfaceType = "relay"

		interfaceConfig := strings.Split(result[i], ":")
		iface, _ := net.InterfaceByName(interfaceConfig[0])
		ethIf.intNet = iface
		ethIf.Name = iface.Name
		ethIf.listenPort = bootp_client
		interfaceIP, _ := iface.Addrs()
		for _, ip := range interfaceIP {
			ip := ip
			listenIP, NetIP, _ := net.ParseCIDR(ip.String())
			if IsIPv6(listenIP) {
				ethIf.Ipv6 = listenIP
				continue
			}
			if IsIPv4(listenIP) {
				ethIf.Ipv4 = listenIP
			}
			ethIf.layer2 = append(ethIf.layer2, NetIP)
			ethIf.relayIP = net.ParseIP(interfaceConfig[1])
		}
		d.intsNet = append(d.intsNet, ethIf)
	}
}
