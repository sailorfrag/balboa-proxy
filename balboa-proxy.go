package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"inet.af/tcpproxy"
)

var (
	dataListenAddr = flag.String("data-addr", "[::0]:4257", "Listen address for Balboa app connections")
	discoveryPort  = flag.Int("discovery-port", 30303, "Port to listen for Balboa app discovery broadcasts")
	discoveryName  = flag.String("discovery-name", "BWGSPA", "Hostname to report in discovery responses")
	discoveryMAC   = flag.String("discovery-mac", "00-15-27-00-00-00", "MAC address to report in discovery responses")
	forwardAddr    = flag.String("forward-addr", "", "Address of Spa module")
)

func main() {
	flag.Parse()

	if *forwardAddr == "" {
		_, _ = fmt.Fprintln(os.Stderr, "--forward-addr must be set")
		os.Exit(1)
	}

	var p tcpproxy.Proxy
	p.AddRoute(*dataListenAddr, tcpproxy.To(*forwardAddr))
	if err := p.Start(); err != nil {
		log.Printf("start proxy: %v", err)
		return
	}

	discoCtx, discoCancel := context.WithCancel(context.Background())
	defer discoCancel()

	go func() {
		if err := listenDiscovery(discoCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("listenDiscovery: %v", err)
		}
		p.Close()
	}()

	if err := p.Wait(); err != nil {
		log.Printf("proxy: %v", err)
	}
}

func listenDiscovery(ctx context.Context) error {
	var lpc net.ListenConfig

	pc, err := lpc.ListenPacket(ctx, "udp", fmt.Sprintf(":%d", *discoveryPort))
	if err != nil {
		return err
	}
	defer pc.Close()

	response := []byte(fmt.Sprintf("%-10s\r\n%s\r\n", *discoveryName, *discoveryMAC))

	buf := make([]byte, 1024)
	for ctx.Err() == nil {
		_, addr, err := pc.ReadFrom(buf)
		if err != nil {
			log.Printf("Discovery read error: %v", err)
			continue
		}

		log.Printf("Discovery request from %s", addr)

		_, err = pc.WriteTo(response, addr)
		if err != nil {
			log.Printf("Discovery write (%s) error: %v", addr, err)
			continue
		}
	}
	return ctx.Err()
}
