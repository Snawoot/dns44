package main

import (
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Snawoot/dns44/dnsproxy"
	"github.com/Snawoot/dns44/mapping"
	"github.com/Snawoot/dns44/pool"
)

const (
	ProgName = "DNS44"
)

type addrPort struct {
	value netip.AddrPort
}

func (a *addrPort) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.value.String()
}

func (a *addrPort) Set(arg string) error {
	parsed, err := netip.ParseAddrPort(arg)
	if err != nil {
		return fmt.Errorf("unable to parse address-port %q: %w", arg, err)
	}
	a.value = parsed
	return nil
}

type addressRange struct {
	rangeStart netip.Addr
	rangeEnd   netip.Addr
}

func (r *addressRange) String() string {
	if r == nil {
		return "<nil>-<nil>"
	}
	return fmt.Sprintf("%s-%s", r.rangeStart, r.rangeEnd)
}

func (r *addressRange) Set(arg string) error {
	parts := strings.SplitN(arg, "-", 2)
	if len(parts) < 2 {
		return fmt.Errorf("bad number of components in range. expected 2, got %d", len(parts))
	}
	start, err := netip.ParseAddr(parts[0])
	if err != nil {
		return fmt.Errorf("unable to parse start address: %w", err)
	}
	end, err := netip.ParseAddr(parts[1])
	if err != nil {
		return fmt.Errorf("unable to parse end address: %w", err)
	}
	r.rangeStart = start
	r.rangeEnd = end
	return nil
}

var (
	home, _   = os.UserHomeDir()
	defDBPath = filepath.Join(home, ".dns44", "db")
	version   = "undefined"

	showVersion    = flag.Bool("version", false, "show program version and exit")
	dnsBindAddress = &addrPort{
		value: netip.MustParseAddrPort("127.0.0.1:4453"),
	}
	dnsUpstream = flag.String("dns-upstream", "1.1.1.1", "upstream DNS server")
	ipRange     = &addressRange{
		rangeStart: netip.MustParseAddr("172.24.0.0"),
		rangeEnd:   netip.MustParseAddr("172.24.255.255"),
	}
	dbPath = flag.String("db-path", defDBPath, "path to database")
	ttl    = flag.Uint("ttl", 900, "TTL for responses")
)

func init() {
	flag.Var(ipRange, "ip-range", "IP address range where all DNS requests are mapped")
	flag.Var(dnsBindAddress, "dns-bind-address", "DNS service bind address")
}

func run() int {
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return 0
	}

	ipPool, err := pool.New(ipRange.rangeStart, ipRange.rangeEnd)
	if err != nil {
		log.Fatalf("unable to create IP pool: %v", err)
	}

	ensureDir(*dbPath)
	mapping, err := mapping.New(*dbPath, ipPool)
	if err != nil {
		log.Fatalf("mapping init failed: %v", err)
	}
	defer mapping.Close()

	dnsCfg := dnsproxy.Config{
		ListenAddr: dnsBindAddress.value,
		Upstream:   *dnsUpstream,
		Mapper:     mapping,
		TTL:        uint32(*ttl),
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
