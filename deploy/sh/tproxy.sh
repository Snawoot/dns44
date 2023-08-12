#!/bin/sh

ip route add local 172.24.0.0/16 dev lo src 127.0.0.1
iptables -t mangle -I PREROUTING -d 172.24.0.0/16 -p udp -j TPROXY --on-port 4480 --on-ip 127.0.0.1 --tproxy-mark 44
iptables -t mangle -I PREROUTING -d 172.24.0.0/16 -p tcp -j TPROXY --on-port 4480 --on-ip 127.0.0.1 --tproxy-mark 44
