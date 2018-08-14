package main

import (
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

func init() {
	if asn == "" {
		asn = "65000"
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
