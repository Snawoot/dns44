// Package dnsproxy is responsible for the DNS proxy server that will redirect
// specified domains to the SNI proxy.
package dnsproxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
)

// DNSProxy is a struct that manages the DNS proxy server.  This server's
// purpose is to redirect queries to a specified SNI proxy.
type DNSProxy struct {
	proxy  *proxy.Proxy
	mapper Mapper
	ttl    uint32
}

// type check
var _ io.Closer = (*DNSProxy)(nil)

// New creates a new instance of *DNSProxy.
func New(cfg *Config) (d *DNSProxy, err error) {
	proxyConfig, err := createProxyConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dnsproxy: invalid configuration: %w", err)
	}

	d = &DNSProxy{
		proxy: &proxy.Proxy{
			Config: proxyConfig,
		},
		mapper: cfg.Mapper,
		ttl:    cfg.TTL,
	}
	d.proxy.Config.RequestHandler = d.requestHandler

	return d, nil
}

// Start starts the DNSProxy server.
func (d *DNSProxy) Start() (err error) {
	err = d.proxy.Start()
	return err
}

// Close implements the [io.Closer] interface for DNSProxy.
func (d *DNSProxy) Close() (err error) {
	err = d.proxy.Stop()
	return err
}

// requestHandler is a [proxy.RequestHandler] implementation which purpose is
// to implement the actual mapping logic.
func (d *DNSProxy) requestHandler(p *proxy.Proxy, ctx *proxy.DNSContext) (err error) {
	qName := ctx.Req.Question[0].Name
	qType := ctx.Req.Question[0].Qtype

	if qType == dns.TypeA || qType == dns.TypeAAAA {
		if err := d.rewrite(qName, qType, ctx); err != nil {
			return fmt.Errorf("rewrite error: %w", err)
		}
		return nil
	}

	return p.Resolve(ctx)
}

// rewrite rewrites the specified query and redirects the response to the
// configured IP addresses.
func (d *DNSProxy) rewrite(qName string, qType uint16, ctx *proxy.DNSContext) error {
	resp := &dns.Msg{}
	resp.SetReply(ctx.Req)
	resp.Compress = true

	domainName := strings.TrimSuffix(strings.ToLower(qName), ".")
	clientKey := "<bogus>"
	clientAddrPort, err := netip.ParseAddrPort(ctx.Addr.String())
	if err != nil {
		log.Println("can't parse ctx.Addr %q: %v", ctx.Addr.String(), err)
	} else {
		clientKey = clientAddrPort.Addr().String()
	}
	answerAddress, err := d.mapper.EnsureMapping(clientKey, domainName, time.Duration(d.ttl+1)*time.Second)
	if err != nil {
		return fmt.Errorf("mapping error: %w", err)
	}

	hdr := dns.RR_Header{
		Name:   qName,
		Rrtype: qType,
		Class:  dns.ClassINET,
		Ttl:    d.ttl,
	}

	switch qType {
	case dns.TypeA:
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: hdr,
			A:   answerAddress.AsSlice(),
		})
	case dns.TypeAAAA:
	}

	ctx.Res = resp
	return nil
}

// createProxyConfig creates DNS proxy configuration.
func createProxyConfig(cfg *Config) (proxyConfig proxy.Config, err error) {
	upstreamCfg, err := proxy.ParseUpstreamsConfig([]string{cfg.Upstream}, nil)
	if err != nil {
		return proxyConfig, fmt.Errorf("failed to parse upstream %s: %w", cfg.Upstream, err)
	}

	ip := net.IP(cfg.ListenAddr.Addr().AsSlice())

	udpPort := &net.UDPAddr{
		IP:   ip,
		Port: int(cfg.ListenAddr.Port()),
	}
	tcpPort := &net.TCPAddr{
		IP:   ip,
		Port: int(cfg.ListenAddr.Port()),
	}

	proxyConfig.UDPListenAddr = []*net.UDPAddr{udpPort}
	proxyConfig.TCPListenAddr = []*net.TCPAddr{tcpPort}
	proxyConfig.UpstreamConfig = upstreamCfg

	return proxyConfig, nil
}
