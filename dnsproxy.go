package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var (
	trr = []string{
		"dns.digitale-gesellschaft.ch",
		"odvr.nic.cz",
		"dns.njal.la",
		"mozilla.cloudflare-dns.com",
	}
	cache4     = NewCache(3600 * time.Second)
	cache6     = NewCache(3600 * time.Second)
	httpClient = &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyurl),
		DialContext: (&net.Dialer{
			Timeout:   500 * time.Millisecond,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           10,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  2 * time.Second,
		ResponseHeaderTimeout:  2 * time.Second,
		DisableKeepAlives:      false,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 4096,
	}}
)

func DoH(qName string, trr string, ipv6 bool) ([]net.IP, []string, error) {
	var (
		ips    []net.IP
		cnames []string
	)

	m := new(dns.Msg)
	if ipv6 {
		m.SetQuestion(qName, dns.TypeAAAA)
	} else {
		m.SetQuestion(qName, dns.TypeA)
	}
	m.SetEdns0(4096, true)
	out, err := m.Pack()
	if err != nil {
		return ips, cnames, errors.New("Error packing request")
	}

	req, err := http.NewRequest("POST", "https://"+trr+":443/dns-query", bytes.NewReader(out))
	if err != nil {
		return ips, cnames, errors.New("Invalid HTTP request")
	}
	req.Header.Set("content-type", "application/dns-message")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ips, cnames, errors.New("Error doing HTTP request")
	}

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ips, cnames, errors.New("Error reading response body")
	}

	rm := new(dns.Msg)
	if err := rm.Unpack(buf); err != nil {
		return ips, cnames, errors.New("Error unpacking response")
	}

	if rm.Rcode != dns.RcodeSuccess {
		return ips, cnames, errors.New("Error code in DNS response")
	}

	for _, ansa := range rm.Answer {
		switch ansb := ansa.(type) {
		case *dns.A:
			ips = append(ips, ansb.A)
		case *dns.AAAA:
			ips = append(ips, ansb.AAAA)
		case *dns.CNAME:
			cnames = append(cnames, ansb.Target)
		case *dns.DNAME:
			cnames = append(cnames, ansb.Target)
		}
	}

	if len(ips) == 0 {
		return ips, cnames, errors.New("No IP addresses in response")
	}

	return ips, cnames, nil
}

func retNull(m *dns.Msg, qName string) {
	m.Answer = nil
	newRR, err := dns.NewRR(fmt.Sprintf("%s A %s", qName, "0.0.0.0"))
	if err == nil {
		m.Answer = append(m.Answer, newRR)
	}
	newRR, err = dns.NewRR(fmt.Sprintf("%s AAAA %s", qName, "0000:0000:0000:0000:0000:0000:0000:0000"))
	if err == nil {
		m.Answer = append(m.Answer, newRR)
	}
}

func addIP(m *dns.Msg, qName string, ip net.IP) {
	rr, err := dns.NewRR(fmt.Sprintf("%s A %s", qName, ip.String()))
	if err == nil {
		m.Answer = append(m.Answer, rr)
	}
}

func addIPv6(m *dns.Msg, qName string, ip net.IP) {
	rr, err := dns.NewRR(fmt.Sprintf("%s AAAA %s", qName, ip.String()))
	if err == nil {
		m.Answer = append(m.Answer, rr)
	}
}

func parseQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA, dns.TypeAAAA:

			qName := strings.ToLower(q.Name)
			if strings.HasSuffix(qName, "youtube.com.") {
				switch qName {
				case "youtube.com.":
					qName = "restrictmoderate.youtube.com."
				case "www.youtube.com.":
					qName = "restrictmoderate.youtube.com."
				case "m.youtube.com.":
					qName = "restrictmoderate.youtube.com."
				case "youtubei.googleapis.com.":
					qName = "restrictmoderate.youtube.com."
				case "youtube.googleapis.com.":
					qName = "restrictmoderate.youtube.com."
				case "www.youtube-nocookie.com.":
					qName = "restrictmoderate.youtube.com."
				case "consent.youtube.com.":
					qName = "consent.youtube.com."
				default:
					retNull(m, q.Name)
					return
				}
			}
			if strings.HasSuffix(qName, "tgragnato.it.") {
				addIPv6(m, q.Name, net.ParseIP("fd76:abcd:ef90::"))
				addIP(m, q.Name, net.ParseIP("172.16.31.0"))
				return
			}

			if q.Qtype == dns.TypeAAAA {
				if data, found := cache6.Get(qName); found {
					for i := range data {
						addIPv6(m, q.Name, data[i])
					}
					return
				}
			} else {
				if data, found := cache4.Get(qName); found {
					for i := range data {
						addIP(m, q.Name, data[i])
					}
					return
				}
			}

			if !checkQuery(qName) {
				retNull(m, q.Name)
				return
			}

			ips, cnames, err := DoH(qName, "dns4torpnlfs2ifuz2s2yf3fc7rdmsbhm6rw75euj35pac6ap25zgqad.onion", q.Qtype == dns.TypeAAAA)
			if err != nil {
				for i := range trr {
					if ips, cnames, err = DoH(qName, trr[i], q.Qtype == dns.TypeAAAA); err == nil {
						break
					}
				}
			}
			if err != nil {
				return
			}

			for _, cname := range cnames {
				if !checkQuery(cname) && !strings.HasSuffix(cname, "cloudfront.net.") {
					retNull(m, q.Name)
					go cache4.Set(qName, net.ParseIP("0.0.0.0"))
					go cache6.Set(qName, net.ParseIP("0000:0000:0000:0000:0000:0000:0000:0000"))
					return
				}
			}

			for i := range ips {
				if !checkIP(ips[i]) {
					retNull(m, q.Name)
					go cache4.Set(qName, net.ParseIP("0.0.0.0"))
					go cache6.Set(qName, net.ParseIP("0000:0000:0000:0000:0000:0000:0000:0000"))
					return
				} else if q.Qtype == dns.TypeAAAA {
					addIPv6(m, q.Name, ips[i])
					go cache6.Set(qName, ips[i])
				} else {
					addIP(m, q.Name, ips[i])
					go cache4.Set(qName, ips[i])
				}
			}

			go IncDNS(strings.ToLower(q.Name[:len(q.Name)-1]))
		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	switch r.Opcode {
	case dns.OpcodeQuery:
		parseQuery(m)
	}

	w.WriteMsg(m)
}
