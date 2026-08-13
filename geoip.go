package main

import (
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

type GeoIP struct{ reader *geoip2.Reader }

func OpenGeoIP(path string) *GeoIP {
	reader, err := geoip2.Open(path)
	if err != nil {
		return &GeoIP{}
	}
	return &GeoIP{reader: reader}
}

func (g *GeoIP) Close() error {
	if g.reader == nil {
		return nil
	}
	return g.reader.Close()
}

func (g *GeoIP) Country(address string) (string, string) {
	if g.reader == nil {
		return "UN", "🇺🇳"
	}
	ip := net.ParseIP(address)
	if ip == nil {
		ips, err := net.LookupIP(address)
		if err != nil || len(ips) == 0 {
			return "UN", "🇺🇳"
		}
		ip = ips[0]
	}
	record, err := g.reader.Country(ip)
	if err != nil || record.Country.IsoCode == "" {
		return "UN", "🇺🇳"
	}
	code := strings.ToUpper(record.Country.IsoCode)
	return code, countryFlag(code)
}

func countryFlag(code string) string {
	if len(code) != 2 {
		return "🇺🇳"
	}
	runes := []rune(strings.ToUpper(code))
	return string([]rune{runes[0] - 'A' + 0x1F1E6, runes[1] - 'A' + 0x1F1E6})
}
