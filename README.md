# configuration

create a config file config.ini

```
[interfaces]
#Interfaces that act as dhcp server
listen=eth1
#Interface:Relay ip mean dhcp request received on this interface will be forwarded to the relay address.
relay=eth1.2:172.20.0.1,eth1.3:172.21.0.1

[network 192.168.1.0]
dns=8.8.8.8,8.8.4.4
next_hop=
gateway=192.168.1.1
dhcp_start=192.168.1.10
domain-name=iastigmate.org
dhcp_max_lease_time=30
dhcpd=enabled
netmask=255.255.255.0
dhcp_end=192.168.1.254
dhcp_default_lease_time=30
```


# API

## IP2MAC

```
curl http://127.0.0.1:22227/api/v1/dhcp/ip/192.168.0.2 | python -m json.tool
```

```
{
    "result": {
        "ip": "192.168.0.2",
        "mac": "10:1f:74:b2:f6:a5"
    }
}
```

## MAC2IP

```
curl http://127.0.0.1:22227/api/v1/dhcp/mac/10:1f:74:b2:f6:a5 | python -m json.tool
```

```
{
    "result": {
        "ip": "192.168.0.2",
        "mac": "10:1f:74:b2:f6:a5"
    }
}
```

## Release IP

```
curl -X "DELETE" http://127.0.0.1:22227/api/v1/dhcp/mac/10:1f:74:b2:f6:a5 | python -m json.tool
```


## Statistics

```
curl http://127.0.0.1:22227/api/v1/dhcp/stats/eth1.137 | python -m json.tool
```

```
[
   {
        "category": "registration",
        "free": 253,
        "interface": "eth1.137",
        "members": [
            {
                "mac": "10:1f:74:b2:f6:a5",
                "ip": "192.168.0.2"
            }
        ],
        "network": "192.168.0.0/24",
        "options": {
            "optionDomainName": "inlinel2.fabianfence",
            "optionDomainNameServer": "10.10.0.1",
            "optionIPAddressLeaseTime": "123",
            "optionNetBIOSOverTCPIPNameServer": "172.20.135.2",
            "optionRouter": "192.168.0.1",
            "optionSubnetMask": "255.255.255.0"
        }
    }
]
```
