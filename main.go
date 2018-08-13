package main

import (
	"errors"
	"log"
	"net"
	"os"
	"time"

	api "github.com/osrg/gobgp/api"
	"github.com/osrg/gobgp/config"
	"github.com/osrg/gobgp/packet/bgp"
	gobgp "github.com/osrg/gobgp/server"
	"github.com/osrg/gobgp/table"
	"github.com/packethost/packngo/metadata"
	"github.com/vishvananda/netlink"
)

var (
	md5Password = os.Getenv("MD5_PASSWORD")
)

func getAnnouncementIP() (net.IP, *net.IPNet, error) {
	device, err := metadata.GetMetadata()
	if err != nil {
		return nil, nil, err
	}
	addr, ok := device.CustomData["BGP_ANNOUNCE"]

	if !ok {
		return nil, nil, errors.New("BGP_ANNOUNCE not set in customdata")
	}

	ip, ipnet, err := net.ParseCIDR(addr.(string))
	if err != nil {
		return nil, nil, err
	}
	return ip, ipnet, nil
}

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

func addAddr() error {
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return err
	}
	_, ipnet, err := getAnnouncementIP()
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

func ensureAddr() {
	for {
		err := addAddr()
		if err != nil {
			log.Println(err)
		}
		time.Sleep(10 * time.Second)
	}
}

func ensureBGP() {
	s := gobgp.NewBgpServer()
	go s.Serve()

	g := api.NewGrpcServer(s, ":50051")
	go g.Serve()

	privIP, err := getPrivateIP()
	if err != nil {
		log.Println(err)
	}

	annIP, annIPNet, err := getAnnouncementIP()
	if err != nil {
		log.Println(err)
	}
	if annIP == nil || annIPNet == nil {
		log.Println("could not find IP")
		return
	}

	ones, _ := annIPNet.Mask.Size()

	// global configuration
	global := &config.Global{
		Config: config.GlobalConfig{
			As:       65000,
			RouterId: privIP.Gateway.String(),
			Port:     -1, // gobgp won't listen on tcp:179,
		},
	}

	if err := s.Start(global); err != nil {
		log.Fatal(err)
	}

	// neighbor configuration
	n := &config.Neighbor{
		Config: config.NeighborConfig{
			NeighborAddress: privIP.Gateway.String(),
			PeerAs:          65530,
			AuthPassword:    md5Password,
		},
	}

	if err := s.AddNeighbor(n); err != nil {
		log.Fatal(err)
	}

	// add routes
	attrs := []bgp.PathAttributeInterface{
		bgp.NewPathAttributeOrigin(0),
		bgp.NewPathAttributeNextHop(privIP.Address.String()),
		bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{bgp.NewAs4PathParam(bgp.BGP_ASPATH_ATTR_TYPE_SEQ, []uint32{4000, 400000, 300000, 40001})}),
	}
	if _, err := s.AddPath("", []*table.Path{table.NewPath(nil, bgp.NewIPAddrPrefix(uint8(ones), annIP.String()), false, attrs, time.Now(), false)}); err != nil {
		log.Fatal(err)
	}

	log.Println(annIP.String())
}

func main() {
	go ensureBGP()
	ensureAddr()
}
