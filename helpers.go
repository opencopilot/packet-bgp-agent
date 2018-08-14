package main

import (
	"errors"
	"net"

	"github.com/packethost/packngo/metadata"
	"github.com/vishvananda/netlink"
)

func getPrivateIP() (*metadata.AddressInfo, error) {
	device, err := metadata.GetMetadata()
	if err != nil {
		return nil, err
	}
	for _, addr := range device.Network.Addresses {
		if addr.Family == 4 && addr.Management && !addr.Public {
			return &addr, nil
		}
	}
	return nil, errors.New("No IP found")
}

// addAddr adds an IP to the loopback device
func addAddr(ipnet *net.IPNet) error {
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return err
	}

	addr, err := netlink.ParseAddr(ipnet.String())
	if err != nil {
		return err
	}
	err = netlink.AddrReplace(lo, addr)

	return err
}
