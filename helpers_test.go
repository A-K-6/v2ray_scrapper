package main

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

func TestSafeOutputName(t *testing.T) {
	if got, err := safeOutputName("sites/google.txt"); err != nil || got != "sites/google.txt" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	for _, name := range []string{"../secret", "/tmp/secret", "."} {
		if _, err := safeOutputName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}

func TestCountryFiltering(t *testing.T) {
	servers := []ProxyServer{{CountryCode: "US"}, {CountryCode: "DE"}}
	filtered := filterCountries(servers, "us, FR")
	if len(filtered) != 1 || filtered[0].CountryCode != "US" {
		t.Fatalf("filtered=%#v", filtered)
	}
}
