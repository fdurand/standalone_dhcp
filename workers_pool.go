package main

import (
	"context"
	_ "expvar"
	"net"

	"github.com/davecgh/go-spew/spew"
	dhcp "github.com/krolaw/dhcp4"
)

type job struct {
	p        dhcp.Packet
	msgType  dhcp.MessageType
	handler  Handler
	addr     net.Addr
	dst      net.IP
	conn     *serveIfConn
	localCtx context.Context
}

func doWork(id int, jobe job) {
	var ans Answer
	spew.Dump(jobe.conn.ifIndex)
	if ans = jobe.handler.ServeDHCP(jobe.localCtx, jobe.p, jobe.msgType, jobe.addr); ans.D != nil {
		if ans.dhcpType == "relay" {
			switch jobe.msgType {
			case dhcp.Discover:
				sendUnicastDHCP(ans.D, ans.srvIP, ans.SrcIP, jobe.p.GIAddr(), 68, 67)
			case dhcp.Offer:
				client, _ := NewRawClient(ans.Iface)
				client.sendDHCP(ans.MAC, ans.D, ans.IP, ans.SrcIP)
				client.Close()
			case dhcp.Request:
				sendUnicastDHCP(ans.D, ans.srvIP, ans.SrcIP, jobe.p.GIAddr(), 68, 67)
			case dhcp.ACK:
				client, _ := NewRawClient(ans.Iface)
				client.sendDHCP(ans.MAC, ans.D, ans.IP, ans.SrcIP)
				client.Close()
			}
		} else {
			ipStr, _, _ := net.SplitHostPort(jobe.addr.String())
			if !(jobe.p.GIAddr().Equal(net.IPv4zero) && net.ParseIP(ipStr).Equal(net.IPv4zero)) {
				sendUnicastDHCP(ans.D, jobe.addr, jobe.dst, jobe.p.GIAddr(), 0, 0)
			} else {
				client, _ := NewRawClient(ans.Iface)
				client.sendDHCP(ans.MAC, ans.D, ans.IP, ans.SrcIP)
				client.Close()
			}
		}
	}
}
