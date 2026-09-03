package service

import "testing"

func TestSafeOutputName(t *testing.T) {
	if got, err := SafeOutputName("sites/google.txt"); err != nil || got != "sites/google.txt" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	for _, name := range []string{"../secret", "/tmp/secret", "."} {
		if _, err := SafeOutputName(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
