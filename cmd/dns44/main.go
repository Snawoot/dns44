package main

import (
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Snawoot/dns44/dnsproxy"
)

const (
	ProgName = "DNS44"
)

var (
	home, _ = os.UserHomeDir()
	version = "undefined"

	showVersion    = flag.Bool("version", false, "show program version and exit")
	dnsBindAddress = flag.String("dns-bind-address", "127.0.0.1:4453", "DNS service bind address")
	dnsUpstream    = flag.String("dns-upstream", "1.1.1.1", "upstream DNS server")
)

func run() int {
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return 0
	}

	parsedDNSBindAddress, err := netip.ParseAddrPort(*dnsBindAddress)
	if err != nil {
		log.Fatalf("can't parse DNS bind address: %v", err)
	}

	dnsCfg := dnsproxy.Config{
		ListenAddr: parsedDNSBindAddress,
		Upstream:   *dnsUpstream,
	}

	log.Println("Starting DNS server...")
	dnsProxy, err := dnsproxy.New(&dnsCfg)
	if err != nil {
		log.Fatalf("unable to instantiate DNS server: %v", err)
	}

	if err := dnsProxy.Start(); err != nil {
		log.Fatalf("Unable to start DNS server: %v", err)
	}
	defer dnsProxy.Close()
	log.Println("DNS server started.")

	// Subscribe to the OS events.
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
	<-signalChannel

	return 0
}

func ensureDir(path string) {
	if err := os.MkdirAll(path, 0700); err != nil {
		log.Fatalf("failed to create database directory: %v", err)
	}
}

func main() {
	log.Default().SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Default().SetPrefix(strings.ToUpper(ProgName) + ": ")
	log.SetOutput(os.Stderr)
	os.Exit(run())
}
