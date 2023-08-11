# dns44

*\[ DNS-four-to-four \]*

IPv4 to IPv4 mapping DNS server. It's an amalgamation of DNS and transparent TCP/UDP proxy, acting together to distinguish connections by their domain names.

The name of program is an allusion to [DNS64](https://en.wikipedia.org/wiki/IPv6_transition_mechanism#DNS64) technology, which also synthesizes address responses to map connections into desired address space.

DNS component responds to all requests with an IP address picked from some specified (likely private) range. It ensures each requested domain has unique IP address assigned.

Transparent proxy accepts connections which arrive at these "virtual" addresses. Further, proxy restores original domain and destination using 1:1 mapping made by DNS server and communicates connection to real destination.

At this moment only logging functionality is implemented, doing journaling of connections made and domains associated with the cause of these connections. However, it's easy to extend this application, providing some custom dialer with custom connection rules leveraging obtained information about domain names.

## Features

* Covers all TCP and UDP ports.
* Protocol-agnostic. Doesn't rely on SNI, HTTP headers or some other protocol-specific markers of domain name.
* Doesn't require use of Linux kernel's conntrack. It relies on TPROXY which does direct socket dispatch.

## Building

```
make
```

## Running

Application uses IP\_TRANSPARENT socket option, so it needs CAP\_NET\_ADMIN or superuser privileges.

Run daemon:

```
dns44 -dns-bind-address=127.0.0.2:53
```

By default, it serves network `172.24.0.0/16`.

Mark this network as local destination of routing table and enable TPROXY socket dispatch:

```
ip route add local 172.24.0.0/16 dev lo src 127.0.0.1
iptables -t mangle -I PREROUTING -d 172.24.0.0/16 -p tcp -j TPROXY --on-port 4480 --on-ip 127.0.0.1 --tproxy-mark 44
iptables -t mangle -I PREROUTING -d 172.24.0.0/16 -p udp -j TPROXY --on-port 4480 --on-ip 127.0.0.1 --tproxy-mark 44
```

Check if everything is working:

```
wget -q  --dns-servers=127.0.0.2 -O- https://ifconfig.co
```

It should output your external IP address.

Daemon log output should be looking like this:

```
DNS44: 2023/08/11 23:25:02.986839 main.go:144: Starting DNS server...
DNS44: 2023/08/11 23:25:02.987198 main.go:154: DNS server started.
DNS44: 2023/08/11 23:25:02.992157 main.go:162: Starting UDP proxy server...
DNS44: 2023/08/11 23:25:02.992292 main.go:168: UDP proxy server started.
DNS44: 2023/08/11 23:25:02.992336 main.go:170: Starting TCP proxy server...
DNS44: 2023/08/11 23:25:02.992421 main.go:174: TCP proxy server started.
DNS44: 2023/08/11 23:25:05.685669 dnsproxy.go:65: DNS ?AAAA ifconfig.co.
DNS44: 2023/08/11 23:25:05.685671 dnsproxy.go:65: DNS ?A ifconfig.co.
DNS44: 2023/08/11 23:25:05.689584 tcp.go:101: [+] TCP 127.0.0.1:37746 <=> [ifconfig.co(172.24.222.11)]:443
DNS44: 2023/08/11 23:25:05.860303 tcp.go:115: [-] TCP 127.0.0.1:37746 <=> [ifconfig.co(172.24.222.11)]:443
```

Finally, adjust DNS bind address to make sure machines subjected to traffic proxying use this DNS server and ready to forward that private network through machine with dns44 server running. E.g. if your are configuring this on some VPN server, just make sure clients receive correct DNS address where dns44 listens.

## Synopsis

```
$ dns44 -h
Usage of dns44:
  -db-path string
    	path to database (default "/home/user/.dns44/db")
  -debug
    	debug logging
  -dial-timeout duration
    	dial timeout for connection originated by proxy (default 10s)
  -dns-bind-address value
    	DNS service bind address (default 127.0.0.1:4453)
  -dns-upstream string
    	upstream DNS server (default "1.1.1.1")
  -ip-range value
    	IP address range where all DNS requests are mapped (default 172.24.0.0-172.24.255.255)
  -proxy-bind-address value
    	transparent proxy service bind address (default 127.0.0.1:4480)
  -ttl uint
    	TTL for responses (default 900)
  -version
    	show program version and exit
```
