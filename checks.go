package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"strings"
)

var (
	unrouteables [15]*net.IPNet
	blackhole    []net.IP
	blacklist    []string
	whitelist    []string
)

func initCheck() {
	cidrstrings := [15]string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"0.0.0.0/8",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}

	blackholestrings := []string{
		"35.201.103.212",
		"139.45.196.145",
		"139.45.196.206",
		"192.243.59.12",
		"192.243.59.13",
		"192.243.59.20",
		"216.21.13.14",
		"216.21.13.15",
	}

	for i := range cidrstrings {
		_, unrouteables[i], _ = net.ParseCIDR(cidrstrings[i])
	}

	for i := range blackholestrings {
		blackhole = append(blackhole, net.ParseIP(blackholestrings[i]))
	}

	blacklist = populateCheck("/etc/blocked.names")
	whitelist = populateCheck("/etc/allowed.names")

}

func populateCheck(conf string) []string {
	var dNames []string

	buf, err := os.Open(conf)
	if err != nil {
		log.Printf("Error opening file %s", conf)
		return dNames
	}

	defer func() {
		if err = buf.Close(); err != nil {
			log.Printf("Error closing file %s : %s", conf, err.Error())
		}
	}()

	snl := bufio.NewScanner(buf)
	for snl.Scan() {

		if err := snl.Err(); err == nil {
			txt := snl.Text()
			if !strings.HasPrefix(txt, "#") && txt != "" {
				dNames = append(dNames, txt)
			} else {
				log.Printf("Error reading newline in file %s : %s", conf, err.Error())
				break
			}
		}

	}

	return dNames
}

func checkQuery(domain string) bool {
	for i := range whitelist {
		if domain == whitelist[i] {
			return true
		}
	}
	for i := range blacklist {
		if strings.HasSuffix(domain, blacklist[i]) {
			return false
		}
	}
	return true
}

func checkDomain(domain string) bool {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if !checkIP(ip) {
			return false
		}
	}
	return true
}

func checkIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() {
		return false
	}
	for i := range unrouteables {
		if unrouteables[i].Contains(ip) {
			return false
		}
	}
	for i := range blackhole {
		if blackhole[i].Equal(ip) {
			return false
		}
	}
	return true
}
