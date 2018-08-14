package main

import (
	"log"
	"net"
	"time"

	"github.com/osrg/gobgp/config"
	"github.com/osrg/gobgp/packet/bgp"
	"github.com/osrg/gobgp/table"
	"github.com/packethost/packetmetadata/packetmetadata"
	"github.com/packethost/packngo/metadata"

	gobgpApi "github.com/osrg/gobgp/api"
	gobgpServer "github.com/osrg/gobgp/server"
)

// PacketBGPAgent is an agent that reads data in from Packet metadata and controls BGP announcement
type PacketBGPAgent struct {
	BGPServer      *gobgpServer.BgpServer
	BGPGRPCServer  *gobgpApi.Server
	AnnoucementIPs []string
	PrivateIP      *metadata.AddressInfo
	MD5Password    string
}

// NewPacketBGPAgent creates a new PacketBGPAgent
func NewPacketBGPAgent(bgpServer *gobgpServer.BgpServer, grpcServer *gobgpApi.Server, md5Password string) (*PacketBGPAgent, error) {
	privateIP, err := getPrivateIP()
	if err != nil {
		return nil, err
	}

	// global configuration
	global := &config.Global{
		Config: config.GlobalConfig{
			As:       65000,
			RouterId: privateIP.Gateway.String(),
			Port:     -1, // gobgp won't listen on tcp:179,
		},
	}

	if err := bgpServer.Start(global); err != nil {
		return nil, err
	}

	// neighbor configuration
	n := &config.Neighbor{
		Config: config.NeighborConfig{
			NeighborAddress: privateIP.Gateway.String(),
			PeerAs:          65530,
			AuthPassword:    md5Password,
		},
	}

	if err := bgpServer.AddNeighbor(n); err != nil {
		return nil, err
	}

	return &PacketBGPAgent{
		BGPServer:      bgpServer,
		BGPGRPCServer:  grpcServer,
		AnnoucementIPs: []string{},
		PrivateIP:      privateIP,
		MD5Password:    md5Password,
	}, nil
}

// EnsureIPs should be run as a go routine, watches metadata for IPs and adds them to the PacketBGPAgent
func (agent *PacketBGPAgent) EnsureIPs(done chan bool) {
	iterator, err := packetmetadata.Watch()
	if err != nil {
		log.Println(err)
	}

	for {
		select {
		case <-done:
			break
		default:
			res, err := iterator.Next()
			if err != nil {
				log.Println(err)
			}

			annoucementIPs, ok := res.Metadata.Instance.CustomData["BGP_ANNOUNCE"]
			if !ok {
				log.Println("BGP_ANNOUNCE not set")
				continue
			}

			switch a := annoucementIPs.(type) {
			case string:
				agent.AnnoucementIPs = []string{a}
			case []string:
				agent.AnnoucementIPs = a
			default:
				continue
			}
			agent.EnsureBGP()
		}
	}
}

// EnsureBGP adds all IPs in agent.AnnoucementIPs to BGP server
func (agent *PacketBGPAgent) EnsureBGP() error {
	for _, announceIP := range agent.AnnoucementIPs {
		ip, ipnet, err := net.ParseCIDR(announceIP)
		if err != nil {
			return err
		}

		err = addAddr(ipnet)
		if err != nil {
			return err
		}

		ones, _ := ipnet.Mask.Size()

		// add routes
		attrs := []bgp.PathAttributeInterface{
			bgp.NewPathAttributeOrigin(0),
			bgp.NewPathAttributeNextHop(agent.PrivateIP.Address.String()),
			bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{bgp.NewAs4PathParam(bgp.BGP_ASPATH_ATTR_TYPE_SEQ, []uint32{4000, 400000, 300000, 40001})}),
		}
		if _, err := agent.BGPServer.AddPath("", []*table.Path{table.NewPath(nil, bgp.NewIPAddrPrefix(uint8(ones), ip.String()), false, attrs, time.Now(), false)}); err != nil {
			return err
		}

		log.Println(ip.String())
	}
	return nil
}
