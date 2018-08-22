package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gobgpApi "github.com/osrg/gobgp/api"
	gobgpServer "github.com/osrg/gobgp/server"
)

var (
	md5Password = os.Getenv("MD5_PASSWORD")
	asn         = os.Getenv("ASN")
)

const (
	tag = "unknown" // set with -ldflags
)

func init() {
	var printVersion bool
	flag.StringVar(&md5Password, "md5", "", "Specify MD5 password to announce with")
	flag.StringVar(&asn, "asn", "65000", "ASN to announce with")
	flag.BoolVar(&printVersion, "version", false, "print the current version")
	flag.Parse()

	if printVersion {
		fmt.Println(tag)
		os.Exit(0)
	}
}

func main() {
	s := gobgpServer.NewBgpServer()
	go s.Serve()

	g := gobgpApi.NewGrpcServer(s, ":50051")
	go g.Serve()

	agent, err := NewPacketBGPAgent(s, g, md5Password, asn)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("started new bgp agent MD5=%s, ASN=%s \n", md5Password, asn)

	quit := make(chan bool, 1)
	go agent.EnsureIPs(quit)

	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	<-gracefulStop
	log.Println("received stop signal, shutting down")
	quit <- true
	time.Sleep(1 * time.Second)
	os.Exit(0)
}
