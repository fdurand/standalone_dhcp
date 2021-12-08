package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/fdurand/arp"
	"github.com/inverse-inc/packetfence/go/log"
	"github.com/inverse-inc/packetfence/go/sharedutils"
	dhcp "github.com/krolaw/dhcp4"
)

// Broadcast Listener
func (I *Interface) run(ctx context.Context, jobs chan job) {

	ListenAndServeIf(ctx, I, I, jobs)
}

// Unicast listener
func (I *Interface) runUnicast(ctx context.Context, jobs chan job) {

	ListenAndServeIfUnicast(ctx, I, I, jobs)
}

//  Return true is it's a relay interface
func (I *Interface) isRelay() bool {
	if I.InterfaceType == "relay" {
		return true
	}
	return false
}

func (I *Interface) ServeDHCP(ctx context.Context, p dhcp.Packet, msgType dhcp.MessageType, srcIP net.Addr, srvIP net.IP) (answer Answer) {

	var handler DHCPHandler

	Local := false
	options := p.ParseOptions()
	answer.MAC = p.CHAddr()
	answer.SrcIP = I.Ipv4

	ctx = log.AddToLogContext(ctx, "mac", answer.MAC.String())

	// DHCP Relay

	if I.isRelay() {

		switch msgType {

		case dhcp.Discover:
			log.LoggerWContext(ctx).Info("RELAY - DISCOVER " + p.YIAddr().String() + " from " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootRequest)
			p2.SetCHAddr(p.CHAddr())
			p2.SetGIAddr(I.Ipv4)
			p2.SetXId(p.XId())
			p2.SetBroadcast(false)
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.D = p2
			return answer

		case dhcp.Offer:
			var sip net.IP
			for k, v := range p.ParseOptions() {
				if k == dhcp.OptionServerIdentifier {
					sip = v
				}
			}
			log.LoggerWContext(ctx).Info("RELAY - OFFER from " + sip.String() + " " + p.YIAddr().String() + " to " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootReply)
			p2.SetXId(p.XId())
			p2.SetFile(p.File())
			p2.SetFlags(p.Flags())
			p2.SetYIAddr(p.YIAddr())
			p2.SetGIAddr(p.GIAddr())
			p2.SetSIAddr(p.SIAddr())
			p2.SetCHAddr(p.CHAddr())
			p2.SetSecs(p.Secs())
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.IP = p.SIAddr()
			answer.D = p2
			return answer

		case dhcp.Request:
			log.LoggerWContext(ctx).Info("RELAY - REQUEST " + p.YIAddr().String() + " from " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootRequest)
			p2.SetCHAddr(p.CHAddr())
			p2.SetFile(p.File())
			p2.SetCIAddr(p.CIAddr())
			p2.SetSIAddr(p.SIAddr())
			p2.SetGIAddr(I.Ipv4)
			p2.SetXId(p.XId())
			p2.SetBroadcast(false)
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.D = p2
			return answer

		case dhcp.ACK:
			var sip net.IP
			for k, v := range p.ParseOptions() {
				if k == dhcp.OptionServerIdentifier {
					sip = v
				}
			}
			log.LoggerWContext(ctx).Info("RELAY - ACK from " + sip.String() + " " + p.YIAddr().String() + " to " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootReply)
			p2.SetXId(p.XId())
			p2.SetFile(p.File())
			p2.SetFlags(p.Flags())
			p2.SetSIAddr(p.SIAddr())
			p2.SetYIAddr(p.YIAddr())
			p2.SetGIAddr(p.GIAddr())
			p2.SetCHAddr(p.CHAddr())
			p2.SetSecs(p.Secs())
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.D = p2
			return answer

		case dhcp.NAK:
			log.LoggerWContext(ctx).Info("RELAY - NAK from " + p.YIAddr().String() + " from " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootReply)
			p2.SetXId(p.XId())
			p2.SetFile(p.File())
			p2.SetFlags(p.Flags())
			p2.SetSIAddr(p.SIAddr())
			p2.SetYIAddr(p.YIAddr())
			p2.SetGIAddr(p.GIAddr())
			p2.SetCHAddr(p.CHAddr())
			p2.SetSecs(p.Secs())
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.D = p2
			return answer

		case dhcp.Release, dhcp.Decline:
			log.LoggerWContext(ctx).Info("RELAY - RELEASE/DECLINE from " + p.SIAddr().String() + " " + p.YIAddr().String() + " to " + p.CHAddr().String())
			p2 := dhcp.NewPacket(dhcp.BootRequest)
			p2.SetCHAddr(p.CHAddr())
			p2.SetFile(p.File())
			p2.SetCIAddr(p.CIAddr())
			p2.SetSIAddr(p.SIAddr())
			p2.SetGIAddr(I.Ipv4)
			p2.SetXId(p.XId())
			p2.SetBroadcast(false)
			for k, v := range p.ParseOptions() {
				p2.AddOption(k, v)
			}
			answer.D = p2
			return answer
		}
		return answer
	}

	// Detect the handler to use (config)
	for _, v := range I.network {

		// Case of a l2 dhcp request
		if v.dhcpHandler.layer2 && (p.GIAddr().Equal(net.IPv4zero) || v.network.Contains(p.CIAddr())) {

			// Case we are in L3
			if !p.CIAddr().Equal(net.IPv4zero) && !v.network.Contains(p.CIAddr()) {
				continue
			}
			handler = *v.dhcpHandler
			break

		}
		// Case dhcprequest from an already assigned l3 ip address
		if p.GIAddr().Equal(net.IPv4zero) && v.network.Contains(p.CIAddr()) {
			handler = *v.dhcpHandler
			break
		}

		if (!p.GIAddr().Equal(net.IPv4zero) && v.network.Contains(p.GIAddr())) || v.network.Contains(p.CIAddr()) {
			handler = *v.dhcpHandler
			break
		}
	}

	if len(handler.ip) == 0 {
		return answer
	}
	defer recoverName(options)

	log.LoggerWContext(ctx).Debug(p.CHAddr().String() + " " + msgType.String() + " xID " + sharedutils.ByteToString(p.XId()))

	id, _ := GlobalTransactionLock.Lock()

	cacheKey := p.CHAddr().String() + " " + msgType.String() + " xID " + sharedutils.ByteToString(p.XId())
	if _, found := GlobalTransactionCache.Get(cacheKey); found {
		log.LoggerWContext(ctx).Debug("Not answering to packet. Already in progress")
		GlobalTransactionLock.Unlock(id)
		return answer
	} else {
		GlobalTransactionCache.Set(cacheKey, 1, time.Duration(1)*time.Second)
		GlobalTransactionLock.Unlock(id)
	}

	prettyType := "DHCP" + strings.ToUpper(msgType.String())
	clientMac := p.CHAddr().String()
	clientHostname := string(options[dhcp.OptionHostName])

	switch msgType {

	case dhcp.Discover:
		firstTry := true
		log.LoggerWContext(ctx).Info("DHCPDISCOVER from " + clientMac + " (" + clientHostname + ")")
		var free int
		free = -1
		// Search in the cache if the mac address already get assigned
		if x, found := handler.hwcache.Get(p.CHAddr().String()); found {
			log.LoggerWContext(ctx).Debug("Found in the cache that a IP has already been assigned")
			// Test if we find the the mac address at the index
			_, returnedMac, err := handler.available.GetMACIndex(uint64(x.(int)))
			if returnedMac == p.CHAddr().String() {
				free = x.(int)
			} else if returnedMac == FreeMac {
				// The index is free use it
				handler.hwcache.Delete(p.CHAddr().String())
				// Reserve the ip
				err, returnedMac = handler.available.ReserveIPIndex(uint64(x.(int)), p.CHAddr().String())
				if err != nil && returnedMac == p.CHAddr().String() {
					free = x.(int)
				} else {
					// Something went wrong to reserve the ip retry
					goto retry
				}
				// The ip asked is not the one we have retry
			} else {
				goto retry
			}

			// 5 seconds to send a request
			err = handler.hwcache.Replace(p.CHAddr().String(), free, time.Duration(5)*time.Second)
			if err != nil {
				return answer
			}
			goto reply
		}

	retry:
		// Search for the next available ip in the pool
		if handler.available.FreeIPsRemaining() > 0 {
			var element uint32
			// Check if the device request a specific ip
			if p.ParseOptions()[50] != nil && firstTry {
				log.LoggerWContext(ctx).Debug("Attempting to use the IP requested by the device")
				element = uint32(binary.BigEndian.Uint32(p.ParseOptions()[50])) - uint32(binary.BigEndian.Uint32(handler.start.To4()))
				// Test if we find the the mac address at the index
				_, returnedMac, err := handler.available.GetMACIndex(uint64(element))
				if returnedMac == p.CHAddr().String() {
					log.LoggerWContext(ctx).Debug("The IP asked by the device is available in the pool")
					free = int(element)
				} else if returnedMac == FreeMac {
					// The ip is free use it
					err, returnedMac = handler.available.ReserveIPIndex(uint64(element), p.CHAddr().String())
					// Reserve the ip
					if err != nil && returnedMac == p.CHAddr().String() {
						log.LoggerWContext(ctx).Debug("The IP asked by the device is available in the pool")
						free = int(element)
					}
				} else {
					// The ip is not available
					firstTry = false
					goto retry
				}
			}

			// If we still haven't found an IP address to offer, we get the next one
			if free == -1 {
				log.LoggerWContext(ctx).Debug("Grabbing next available IP")
				freeu64, _, err := handler.available.GetFreeIPIndex(p.CHAddr().String())

				if err != nil {
					log.LoggerWContext(ctx).Error("Unable to get free IP address, DHCP pool is full")
					return answer
				}
				free = int(freeu64)
			}

			// Lock it
			handler.hwcache.Set(p.CHAddr().String(), free, time.Duration(5)*time.Second)
			handler.xid.Set(sharedutils.ByteToString(p.XId()), 0, time.Duration(5)*time.Second)
			var inarp bool
			// Ping the ip address
			inarp = false
			// Layer 2 test (arp cache)
			if Local {
				mac := arp.Search(dhcp.IPAdd(handler.start, free).String())
				if mac != "" && mac != FreeMac {
					if p.CHAddr().String() != mac {
						log.LoggerWContext(ctx).Info(p.CHAddr().String() + " in arp table Ip " + dhcp.IPAdd(handler.start, free).String() + " is already own by " + mac)
						inarp = true
					}
				}
			}
			// Layer 3 Test
			pingreply := sharedutils.Ping(setOptionServerIdentifier(srvIP, handler.ip).To4(), dhcp.IPAdd(handler.start, free), I.Name, 1)
			if pingreply || inarp {
				// Found in the arp cache or able to ping it
				ipaddr := dhcp.IPAdd(handler.start, free)
				log.LoggerWContext(ctx).Info(p.CHAddr().String() + " Ip " + ipaddr.String() + " already in use, trying next")
				// Added back in the pool since it's not the dhcp server who gave it
				handler.hwcache.Delete(p.CHAddr().String())

				firstTry = false

				log.LoggerWContext(ctx).Info("Temporarily declaring " + ipaddr.String() + " as unusable")
				// Reserve with a fake mac
				handler.available.ReserveIPIndex(uint64(free), FakeMac)
				// Put it back into the available IPs in 10 minutes
				go func(ctx context.Context, free int, ipaddr net.IP) {
					time.Sleep(10 * time.Minute)
					log.LoggerWContext(ctx).Info("Releasing previously pingable IP " + ipaddr.String() + " back into the pool")
					handler.available.FreeIPIndex(uint64(free))
				}(ctx, free, ipaddr)
				free = 0
				goto retry
			}
			// 5 seconds to send a request
			handler.hwcache.Set(p.CHAddr().String(), free, time.Duration(5)*time.Second)
			handler.xid.Replace(sharedutils.ByteToString(p.XId()), 1, time.Duration(5)*time.Second)
		} else {
			log.LoggerWContext(ctx).Info(p.CHAddr().String() + " Nak No space left in the pool ")
			return answer
		}

	reply:

		answer.IP = dhcp.IPAdd(handler.start, free)
		// Add options on the fly
		var GlobalOptions dhcp.Options
		var options = make(map[dhcp.OptionCode][]byte)
		for key, value := range handler.options {
			if key == dhcp.OptionDomainNameServer || key == dhcp.OptionRouter {
				options[key] = ShuffleIP(value, int64(p.CHAddr()[5]))
			} else {
				options[key] = value
			}
		}
		GlobalOptions = options
		leaseDuration := handler.leaseDuration

		log.LoggerWContext(ctx).Info("DHCPOFFER on " + answer.IP.String() + " to " + clientMac + " (" + clientHostname + ")")

		answer.D = dhcp.ReplyPacket(p, dhcp.Offer, handler.ip.To4(), answer.IP, leaseDuration,
			GlobalOptions.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))

		return answer

	case dhcp.Request, dhcp.Inform:
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}
		log.LoggerWContext(ctx).Info(prettyType + " for " + reqIP.String() + " from " + clientMac + " (" + clientHostname + ")")
		cacheKey := p.CHAddr().String() + " " + msgType.String() + " xID " + sharedutils.ByteToString(p.XId())
		// In the event of a DHCPREQUEST, we do not reply if we're not the server ID in the request
		serverIdBytes := options[dhcp.OptionServerIdentifier]
		if len(serverIdBytes) == 4 {
			serverId := net.IPv4(serverIdBytes[0], serverIdBytes[1], serverIdBytes[2], serverIdBytes[3])
			if !serverId.Equal(handler.ip.To4()) {
				log.LoggerWContext(ctx).Debug(fmt.Sprintf("Not replying to %s because this server didn't perform the offer (offered by %s, we are %s)", prettyType, serverId, handler.ip.To4()))
				return Answer{}
			}
		}

		answer.IP = reqIP

		var Reply bool
		var Index int

		// Valid IP
		if len(reqIP) == 4 && !reqIP.Equal(net.IPv4zero) {
			// Requested IP is in the pool ?
			if leaseNum := dhcp.IPRange(handler.start, reqIP) - 1; leaseNum >= 0 && leaseNum < handler.leaseRange {
				// Requested IP is in the cache ?
				if index, found := handler.hwcache.Get(p.CHAddr().String()); found {
					// Requested IP is equal to what we have in the cache ?
					if dhcp.IPAdd(handler.start, index.(int)).Equal(reqIP) {
						id, _ := GlobalTransactionLock.Lock()
						if _, found = RequestGlobalTransactionCache.Get(cacheKey); found {
							log.LoggerWContext(ctx).Debug("Not answering to REQUEST. Already processed")
							Reply = false
							GlobalTransactionLock.Unlock(id)
							return answer
						} else {
							_, returnedMac, _ := handler.available.GetMACIndex(uint64(index.(int)))
							if returnedMac == p.CHAddr().String() {
								Reply = true
								Index = index.(int)
							} else {
								Reply = false
							}
							RequestGlobalTransactionCache.Set(cacheKey, 1, time.Duration(1)*time.Second)
							GlobalTransactionLock.Unlock(id)
						}
						// So remove the ip from the cache
					} else {
						Reply = false
						log.LoggerWContext(ctx).Info(p.CHAddr().String() + " Asked for an IP " + reqIP.String() + " that hasnt been assigned by Offer " + dhcp.IPAdd(handler.start, index.(int)).String() + " xID " + sharedutils.ByteToString(p.XId()))
						if index, found = handler.xid.Get(string(binary.BigEndian.Uint32(p.XId()))); found {
							if index.(int) == 1 {
								handler.hwcache.Delete(p.CHAddr().String())
							}
						}
					}
				} else {
					// Not in the cache so we don't reply
					log.LoggerWContext(ctx).Debug(fmt.Sprintf("Not replying to %s because this server didn't perform the offer", prettyType))
					return Answer{}
				}
			}

			if Reply {
				var GlobalOptions dhcp.Options
				var options = make(map[dhcp.OptionCode][]byte)
				for key, value := range handler.options {
					if key == dhcp.OptionDomainNameServer || key == dhcp.OptionRouter {
						options[key] = ShuffleIP(value, int64(p.CHAddr()[5]))
					} else {
						options[key] = value
					}
				}
				GlobalOptions = options
				leaseDuration := handler.leaseDuration
				answer.D = dhcp.ReplyPacket(p, dhcp.ACK, handler.ip.To4(), reqIP, leaseDuration,
					GlobalOptions.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))
				// Update Global Caches
				GlobalIpCache.Set(reqIP.String(), p.CHAddr().String(), leaseDuration+(time.Duration(15)*time.Second))
				GlobalMacCache.Set(p.CHAddr().String(), reqIP.String(), leaseDuration+(time.Duration(15)*time.Second))
				// Update the cache
				log.LoggerWContext(ctx).Info("DHCPACK on " + reqIP.String() + " to " + clientMac + " (" + clientHostname + ")")
				handler.hwcache.Set(p.CHAddr().String(), Index, leaseDuration+(time.Duration(15)*time.Second))
				handler.available.ReserveIPIndex(uint64(Index), p.CHAddr().String())
			} else {
				log.LoggerWContext(ctx).Info("DHCPNAK on " + reqIP.String() + " to " + clientMac)
				answer.D = dhcp.ReplyPacket(p, dhcp.NAK, handler.ip.To4(), nil, 0, nil)
			}
			return answer
		}

	case dhcp.Release:
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}
		if leaseNum := dhcp.IPRange(handler.start, reqIP) - 1; leaseNum >= 0 && leaseNum < handler.leaseRange {
			if x, found := handler.hwcache.Get(p.CHAddr().String()); found {
				if leaseNum == x.(int) {
					log.LoggerWContext(ctx).Debug(prettyType + "Found the ip " + reqIP.String() + "in the cache")
					_, returnedMac, _ := handler.available.GetMACIndex(uint64(x.(int)))
					if returnedMac == p.CHAddr().String() {
						log.LoggerWContext(ctx).Info("Temporarily declaring " + reqIP.String() + " as unusable")
						handler.available.ReserveIPIndex(uint64(leaseNum), FakeMac)
						// Put it back into the available IPs in 10 minutes
						go func(ctx context.Context, leaseNum int, reqIP net.IP) {
							time.Sleep(10 * time.Minute)
							log.LoggerWContext(ctx).Info("Releasing previously declined IP " + reqIP.String() + " back into the pool")
							handler.available.FreeIPIndex(uint64(leaseNum))
						}(ctx, leaseNum, reqIP)
						go func(ctx context.Context, x int, reqIP net.IP) {
							handler.hwcache.Delete(p.CHAddr().String())
						}(ctx, x.(int), reqIP)
					}
				} else {
					log.LoggerWContext(ctx).Debug(prettyType + "Found the mac in the cache for but wrong IP")
				}
			}
		}
		log.LoggerWContext(ctx).Info(prettyType + " of " + reqIP.String() + " from " + clientMac)
		return answer
	case dhcp.Decline:
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}

		if leaseNum := dhcp.IPRange(handler.start, reqIP) - 1; leaseNum >= 0 && leaseNum < handler.leaseRange {
			// Remove the mac from the cache
			if x, found := handler.hwcache.Get(p.CHAddr().String()); found {
				if leaseNum == x.(int) {
					log.LoggerWContext(ctx).Debug(prettyType + "Found the ip " + reqIP.String() + "in the cache")
					_, returnedMac, _ := handler.available.GetMACIndex(uint64(x.(int)))
					if returnedMac == p.CHAddr().String() {
						log.LoggerWContext(ctx).Info("Temporarily declaring " + reqIP.String() + " as unusable")
						handler.available.ReserveIPIndex(uint64(leaseNum), FakeMac)
						// Put it back into the available IPs in 10 minutes
						go func(ctx context.Context, leaseNum int, reqIP net.IP) {
							time.Sleep(10 * time.Minute)
							log.LoggerWContext(ctx).Info("Releasing previously declined IP " + reqIP.String() + " back into the pool")
							handler.available.FreeIPIndex(uint64(leaseNum))
						}(ctx, leaseNum, reqIP)
						go func(ctx context.Context, x int, reqIP net.IP) {
							handler.hwcache.Delete(p.CHAddr().String())
						}(ctx, x.(int), reqIP)
					}
				} else {
					log.LoggerWContext(ctx).Debug(prettyType + "Found the mac in the cache for but wrong IP")
				}
			}
		}
		log.LoggerWContext(ctx).Info(prettyType + " of " + reqIP.String() + " from " + clientMac)
		return answer
	}
	log.LoggerWContext(ctx).Info(p.CHAddr().String() + " Nak " + sharedutils.ByteToString(p.XId()))
	answer.D = dhcp.ReplyPacket(p, dhcp.NAK, handler.ip.To4(), nil, 0, nil)
	return answer

}
