package main

import (
	"context"
	_ "expvar"
	"net"
	"strconv"

	"github.com/inverse-inc/packetfence/go/log"
	dhcp "github.com/krolaw/dhcp4"
)

type job struct {
	DHCPpacket dhcp.Packet
	msgType    dhcp.MessageType
	Int        *Interface
	handler    Handler
	clientAddr net.Addr //remote client ip
	srvAddr    net.IP
	localCtx   context.Context
}

func doWork(id int, element job) {
	var ans Answer
	if ans = element.handler.ServeDHCP(element.localCtx, element.DHCPpacket, element.msgType, element.clientAddr, element.srvAddr); ans.D != nil {
		// DHCP Relay
		if element.Int.isRelay() {
			switch element.msgType {
			case dhcp.Discover:
				sendUnicastDHCP(ans.D, element.Int.relayIP, element.Int.Ipv4, element.DHCPpacket.GIAddr(), bootp_client, bootp_server)
			case dhcp.Offer:
				client, err := NewRawClient(element.Int.intNet)
				if err != nil {
					log.LoggerWContext(element.localCtx).Error("Failed to create raw client: " + err.Error())
					return
				}
				client.sendDHCP(ans.MAC, ans.D, ans.IP, element.Int.Ipv4)
				client.Close()
			case dhcp.Request:
				sendUnicastDHCP(ans.D, element.Int.relayIP, element.Int.Ipv4, element.DHCPpacket.GIAddr(), bootp_client, bootp_server)
			case dhcp.ACK:
				client, err := NewRawClient(element.Int.intNet)
				if err != nil {
					log.LoggerWContext(element.localCtx).Error("Failed to create raw client: " + err.Error())
					return
				}
				client.sendDHCP(ans.MAC, ans.D, ans.IP, element.Int.Ipv4)
				client.Close()
			}
		} else {
			// DHCP Server
			ipStr, portStr, _ := net.SplitHostPort(element.clientAddr.String())
			if !(element.DHCPpacket.GIAddr().Equal(net.IPv4zero) && net.ParseIP(ipStr).Equal(net.IPv4zero)) {
				dstPort, _ := strconv.Atoi(portStr)
				sendUnicastDHCP(ans.D, net.ParseIP(ipStr), element.Int.Ipv4, element.DHCPpacket.GIAddr(), bootp_server, dstPort)
			} else {
				client, err := NewRawClient(element.Int.intNet)
				if err != nil {
					log.LoggerWContext(element.localCtx).Error("Failed to create raw client: " + err.Error())
					return
				}
				client.sendDHCP(ans.MAC, ans.D, ans.IP, element.Int.Ipv4)
				client.Close()
			}
		}
	}
}
