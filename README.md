
# Proxy

This is a proxy for TLS and HTTP.
Ingress traffic is captured by nftables.

Most HTTP traffic is upgraded to HTTPS with a redirect.

The SNI field of the TLS is read.
The connection is forwarded to the local socks proxy, or handled directly if a bypass is set.

Filtering is performed at the DNS and SNI layers.


## nftables

```
chain prerouting {
  type nat hook prerouting priority 0; policy accept;
  [...]
  iifname <ifname> ip saddr <ranges> ip daddr != <iface_ip> udp dport domain redirect to :domain
  iifname <ifname> ip saddr <ranges> ip daddr != <iface_ip> tcp dport http redirect to :http
  iifname <ifname> ip saddr <ranges> ip daddr != <iface_ip> tcp dport https redirect to :https
  [...]
}

chain output_<wan> {
  [...]
  tcp sport 1025-65535 tcp dport { http, https } skuid proxy accept
  [...]
}

```
