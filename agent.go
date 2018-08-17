package main

import (
	"context"
	"log"
	"net"
	"strconv"
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
	BGPServer         *gobgpServer.BgpServer
	BGPGRPCServer     *gobgpApi.Server
	AnnoucementIPs    []string
	PrivateIP         *metadata.AddressInfo
	MD5Password       string
	ASN               uint32
	announcementTable map[string][]byte
}

// NewPacketBGPAgent creates a new PacketBGPAgent
func NewPacketBGPAgent(bgpServer *gobgpServer.BgpServer, grpcServer *gobgpApi.Server, md5Password, asn string) (*PacketBGPAgent, error) {
	privateIP, err := getPrivateIP()
	if err != nil {
		return nil, err
	}

	asn64, err := strconv.ParseUint(asn, 10, 32)
	if err != nil {
		return nil, err
	}
	asn32 := uint32(asn64)

	// global configuration
	global := &config.Global{
		Config: config.GlobalConfig{
			As:       asn32,
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
		BGPServer:         bgpServer,
		BGPGRPCServer:     grpcServer,
		AnnoucementIPs:    []string{},
		PrivateIP:         privateIP,
		MD5Password:       md5Password,
		ASN:               asn32,
		announcementTable: make(map[string][]byte),
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
			// handle the done channel, close the metadata connection and break out of the loop
			iterator.Close()
			break
		default:
			// block on iterator.Next(), waiting for a change to come in from metadata
			res, err := iterator.Next()
			if err != nil {
				log.Println(err)
			}

			// If MD5 has changed, set the new one on the agent
			newMD5, ok := res.Metadata.Instance.CustomData["MD5_PASSWORD"]
			if ok && newMD5.(string) != agent.MD5Password {
				agent.MD5Password = newMD5.(string)
				log.Println("changing MD5_PASSWORD")
			}

			// If ASN has changed, set it on the agent, account for if it's specified as string vs int
			incomingASN, ok := res.Metadata.Instance.CustomData["ASN"]
			var newASN uint32
			switch n := incomingASN.(type) {
			case string:
				asn64, err := strconv.ParseUint(n, 10, 32)
				if err != nil {
					log.Println(err)
				}
				newASN = uint32(asn64)
			case float64:
				newASN = uint32(n)
			}

			if ok && newASN != agent.ASN {
				agent.ASN = newASN
				log.Println("changing ASN")
			}

			annoucementIPs, ok := res.Metadata.Instance.CustomData["BGP_ANNOUNCE"]
			if !ok {
				log.Println("BGP_ANNOUNCE not set")
				continue
			}

			switch a := annoucementIPs.(type) {
			case string:
				agent.AnnoucementIPs = []string{a}
			case []interface{}:
				ips := make([]string, 0)
				for i := range a {
					ips = append(ips, a[i].(string))
				}
				agent.AnnoucementIPs = ips
			default:
				continue
			}
			err = agent.EnsureBGP()
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// EnsureBGP adds all IPs in agent.AnnoucementIPs to BGP server
func (agent *PacketBGPAgent) EnsureBGP() error {
	log.Println("ensuring announcement of the following IP blocks: ", agent.AnnoucementIPs)

	for annIP, uuid := range agent.announcementTable {
		exists := false
		for _, annoucementIP := range agent.AnnoucementIPs {
			if exists = annoucementIP == annIP; exists {
				break
			}
		}
		if !exists { // if IP was previously announced but now is removed
			_, err := agent.BGPGRPCServer.DeletePath(context.Background(), &gobgpApi.DeletePathRequest{
				Uuid: uuid,
			})
			if err != nil {
				return err
			}

			delete(agent.announcementTable, annIP)
		}
	}

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
		}
		pathID, err := agent.BGPServer.AddPath("", []*table.Path{table.NewPath(nil, bgp.NewIPAddrPrefix(uint8(ones), ip.String()), false, attrs, time.Now(), false)})
		if err != nil {
			return err
		}

		agent.announcementTable[announceIP] = pathID
	}
	return nil
}
