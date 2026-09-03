package geoip

import "testing"

func TestGeoIPFallbackAndCountryFlag(t *testing.T) {
	geo := OpenGeoIP("/definitely/missing.mmdb")
	defer geo.Close()
	code, flag := geo.Country("not-an-ip")
	if code != "UN" || flag != "🇺🇳" {
		t.Fatalf("country=%s flag=%s", code, flag)
	}
	if countryFlag("US") != "🇺🇸" {
		t.Fatalf("US flag=%s", countryFlag("US"))
	}
}
