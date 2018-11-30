package main

import (
	"encoding/binary"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/go-ini/ini"
	"github.com/inverse-inc/packetfence/go/log"
	"github.com/inverse-inc/packetfence/go/sharedutils"
)

type NodeInfo struct {
	Mac      string
	Status   string
	Category string
}

func InterfaceScopeFromMac(MAC string) string {
	var NetWork string
	if index, found := GlobalMacCache.Get(MAC); found {
		for _, v := range DHCPConfig.intsNet {
			v := v
			for network := range v.network {
				if v.network[network].network.Contains(net.ParseIP(index.(string))) {
					NetWork = v.network[network].network.String()
					if x, found := v.network[network].dhcpHandler.hwcache.Get(MAC); found {
						v.network[network].dhcpHandler.hwcache.Replace(MAC, x.(int), 3*time.Second)
						log.LoggerWContext(ctx).Info(MAC + " removed")
					}
				}
			}
		}
	}
	return NetWork
}

func ShuffleDNS(sec *ini.Section) (r []byte) {
	return Shuffle(sec.Key("dns").String())
}

func ShuffleGateway(sec *ini.Section) (r []byte) {
	return []byte(net.ParseIP(sec.Key("gateway").String()).To4())
}

func Shuffle(addresses string) (r []byte) {
	var array []net.IP
	for _, adresse := range strings.Split(addresses, ",") {
		array = append(array, net.ParseIP(adresse).To4())
	}

	slice := make([]byte, 0, len(array))

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(array) - 1; i > 0; i-- {
		j := random.Intn(i + 1)
		array[i], array[j] = array[j], array[i]
	}
	for _, element := range array {
		elem := []byte(element)
		slice = append(slice, elem...)
	}
	return slice
}

func ShuffleNetIP(array []net.IP, randSrc int64) (r []byte) {

	slice := make([]byte, 0, len(array))

	if randSrc == 0 {
		randSrc = time.Now().UnixNano()
	}
	random := rand.New(rand.NewSource(randSrc))
	for i := len(array) - 1; i > 0; i-- {
		j := random.Intn(i + 1)
		array[i], array[j] = array[j], array[i]
	}
	for _, element := range array {
		elem := []byte(element)
		slice = append(slice, elem...)
	}
	return slice
}

func ShuffleIP(a []byte, randSrc int64) (r []byte) {

	var array []net.IP
	for len(a) != 0 {
		array = append(array, net.IPv4(a[0], a[1], a[2], a[3]).To4())
		_, a = a[0], a[4:]
	}
	return ShuffleNetIP(array, randSrc)
}

func IPsFromRange(ip_range string) (r []net.IP, i int) {
	var iplist []net.IP
	iprange := strings.Split(ip_range, ",")
	if len(iprange) >= 1 {
		for _, rangeip := range iprange {
			ips := strings.Split(rangeip, "-")
			if len(ips) == 1 {
				iplist = append(iplist, net.ParseIP(ips[0]))
			} else {
				start := net.ParseIP(ips[0])
				end := net.ParseIP(ips[1])

				for {
					iplist = append(iplist, net.ParseIP(start.String()))
					if start.Equal(end) {
						break
					}
					sharedutils.Inc(start)
				}
			}
		}
	}
	return iplist, len(iplist)
}

// ExcludeIP remove IP from the pool
func ExcludeIP(dhcpHandler *DHCPHandler, ip_range string) {
	excludeIPs, _ := IPsFromRange(ip_range)

	for _, excludeIP := range excludeIPs {
		if excludeIP != nil {
			// Calculate the position for the dhcp pool
			position := uint32(binary.BigEndian.Uint32(excludeIP.To4())) - uint32(binary.BigEndian.Uint32(dhcpHandler.start.To4()))

			dhcpHandler.available.ReserveIPIndex(uint64(position), FakeMac)
		}
	}
}

func IsIPv4(address net.IP) bool {
	return strings.Count(address.String(), ":") < 2
}

func IsIPv6(address net.IP) bool {
	return strings.Count(address.String(), ":") >= 2
}
