package main

import (
	"log"
	"log/syslog"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

var (
	analytics    = &Analytics{data: map[string]Hits{}}
	express      = &SafeExpress{data: map[string]uint{}}
	asnreader, _ = maxminddb.Open("/var/db/GeoIP/GeoLite2-ASN.mmdb")
	dbreader, _  = maxminddb.Open("/var/db/GeoIP/GeoLite2-Country.mmdb")
	httpclient   = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Minute,
				KeepAlive: time.Millisecond,
				DualStack: true,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConnsPerHost:   10,
			MaxConnsPerHost:       20,
			IdleConnTimeout:       5 * time.Minute,
			ResponseHeaderTimeout: 2 * time.Second,
		},
		Timeout: 5 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

func main() {
	rand.Seed(time.Now().Unix())

	syslogger, err := syslog.Dial("unixgram", "/dev/log", syslog.LOG_INFO, "proxy")
	if err != nil {
		log.Fatalf("Failed to use syslog: %s\n", err.Error())
	}
	log.SetOutput(syslogger)

	if dbreader != nil {
		defer dbreader.Close()
	}
	if asnreader != nil {
		defer asnreader.Close()
	}

	go func() {
		handler := http.DefaultServeMux
		handler.HandleFunc("/", handleHTTPForward)
		handler.HandleFunc("172.16.33.1/", handleAnalytics)
		err := http.ListenAndServe("172.16.33.1:1080", handler)
		if err != nil {
			log.Fatalf("Failed to start server: %s\n", err.Error())
		}
	}()

	listener, err := net.Listen("tcp", "172.16.33.1:1443")
	if err != nil {
		log.Fatalf("Failed to start server: %s\n", err.Error())
	}
	for {
		flow, err := listener.Accept()
		if err != nil {
			continue
		}
		go EstablishFlow(flow)
	}
}
