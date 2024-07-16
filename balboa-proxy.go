package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

var (
	dataListenAddr  = flag.String("data-addr", "[::0]:4257", "Listen address for Balboa app connections")
	discoveryPort   = flag.Int("discovery-port", 30303, "Port to listen for Balboa app discovery broadcasts")
	discoveryName   = flag.String("discovery-name", "BWGSPA", "Hostname to report in discovery responses")
	discoveryMAC    = flag.String("discovery-mac", "00-15-27-00-00-00", "MAC address to report in discovery responses")
	forwardAddrFlag = flag.String("forward-addr", "", "Address of Spa module")
)

func main() {
	flag.Parse()

	if *forwardAddrFlag == "" {
		_, _ = fmt.Fprintln(os.Stderr, "--forward-addr must be set")
		os.Exit(1)
	}
	forwardAddr, err := net.ResolveTCPAddr("tcp", *forwardAddrFlag)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Unable to resolve --forward-addr: %v", err)
		os.Exit(1)
	}

	discoCtx, discoCancel := context.WithCancel(context.Background())
	defer discoCancel()
	forwardCtx, forwardCancel := context.WithCancel(context.Background())
	defer forwardCancel()

	go func() {
		if err := listenDiscovery(discoCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("listenDiscovery: %v", err)
		}
		forwardCancel()
	}()

	if err := listenForward(forwardCtx, forwardAddr); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("listenForward: %v", err)
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

func listenForward(ctx context.Context, forwardAddr net.Addr) error {
	var lpc net.ListenConfig
	ln, err := lpc.Listen(ctx, "tcp", *dataListenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for ctx.Err() == nil {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("Data accept error: %v", err)
			continue
		}

		tc, ok := c.(*net.TCPConn)
		if !ok {
			log.Printf("[unexpected] Incoming connection is not a TCPConn")
		}
		go handleForward(ctx, tc, forwardAddr)
	}

	return ctx.Err()
}

func handleForward(ctx context.Context, c *net.TCPConn, forwardAddr net.Addr) {
	defer c.Close()
	var dial net.Dialer

	log.Printf("Forwarding from %s", c.RemoteAddr())

	fc, err := dial.DialContext(ctx, "tcp", forwardAddr.String())
	if err != nil {
		log.Printf("Unable to dial %s from %s: %v", forwardAddr, c.RemoteAddr(), err)
		return
	}
	defer fc.Close()

	fwd, ok := fc.(*net.TCPConn)
	if !ok {
		log.Printf("[unexpected] Forwarding connection is not a TCPConn")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go pipe(ctx, c, fwd, cancel)
	pipe(ctx, fwd, c, cancel)

	log.Printf("Forward complete %s", c.RemoteAddr())
}

func pipe(ctx context.Context, a, b *net.TCPConn, cancel context.CancelFunc) {
	defer cancel()
	defer b.CloseWrite()
	defer a.CloseRead()

	buf := make([]byte, 4096)
	for ctx.Err() == nil {
		if err := a.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			log.Printf("SetReadDeadline (%v->%v): %v", a.RemoteAddr(), b.RemoteAddr(), err)
			return
		}
		n, err := a.Read(buf)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			continue
		}
		if err != nil {
			log.Printf("Read error (%v->%v): %v", a.RemoteAddr(), b.RemoteAddr(), err)
			return
		}
		if err := b.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			log.Printf("SetWriteDeadline (%v->%v): %v", a.RemoteAddr(), b.RemoteAddr(), err)
			return
		}
		var i int
		for ctx.Err() == nil {
			tx, err := b.Write(buf[i:n])
			if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
				log.Printf("Write error (%v->%v): %v", a.RemoteAddr(), b.RemoteAddr(), err)
				return
			}
			i += tx
		}
	}
}
