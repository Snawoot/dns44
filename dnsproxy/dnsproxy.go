// Package dnsproxy is responsible for the DNS proxy server that will redirect
// specified domains to the SNI proxy.
package dnsproxy

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
)

// defaultTTL is the default TTL for the rewritten records.
const defaultTTL = 60

// DNSProxy is a struct that manages the DNS proxy server.  This server's
// purpose is to redirect queries to a specified SNI proxy.
type DNSProxy struct {
	proxy *proxy.Proxy
}

// type check
var _ io.Closer = (*DNSProxy)(nil)

// New creates a new instance of *DNSProxy.
func New(cfg *Config) (d *DNSProxy, err error) {
	proxyConfig, err := createProxyConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dnsproxy: invalid configuration: %w", err)
	}

	d = &DNSProxy{}
	d.proxy = &proxy.Proxy{
		Config: proxyConfig,
	}
	d.proxy.RequestHandler = d.requestHandler

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
	qName := strings.ToLower(ctx.Req.Question[0].Name)
	qType := ctx.Req.Question[0].Qtype

	// TODO: actually use domain name
	// domainName := strings.TrimSuffix(qName, ".")

	if qType == dns.TypeA || qType == dns.TypeAAAA {
		d.rewrite(qName, qType, ctx)
		return nil
	}

	return p.Resolve(ctx)
}

// rewrite rewrites the specified query and redirects the response to the
// configured IP addresses.
func (d *DNSProxy) rewrite(qName string, qType uint16, ctx *proxy.DNSContext) {
	resp := &dns.Msg{}
	resp.SetReply(ctx.Req)
	resp.Compress = true

	hdr := dns.RR_Header{
		Name:   qName,
		Rrtype: qType,
		Class:  dns.ClassINET,
		Ttl:    defaultTTL,
	}

	switch qType {
	case dns.TypeA:
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: hdr,
			A:   net.IPv4(3, 3, 3, 3),
		})
	case dns.TypeAAAA:
	}

	ctx.Res = resp
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
