package main

import (
	"net"
	"testing"
)

func TestChooseBindHostUsesOverride(t *testing.T) {
	host, usedTailscale := chooseBindHost("192.168.1.50", func() (string, bool) {
		return "100.64.0.10", true
	})
	if host != "192.168.1.50" {
		t.Fatalf("host = %q, want override", host)
	}
	if usedTailscale {
		t.Fatalf("usedTailscale = true, want false for manual override")
	}
}

func TestChooseBindHostPrefersTailscale(t *testing.T) {
	host, usedTailscale := chooseBindHost("", func() (string, bool) {
		return "100.64.0.10", true
	})
	if host != "100.64.0.10" {
		t.Fatalf("host = %q, want tailscale address", host)
	}
	if !usedTailscale {
		t.Fatalf("usedTailscale = false, want true")
	}
}

func TestChooseBindHostFallsBackLocalhost(t *testing.T) {
	host, usedTailscale := chooseBindHost("", func() (string, bool) { return "", false })
	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want localhost", host)
	}
	if usedTailscale {
		t.Fatalf("usedTailscale = true, want false")
	}
}

func TestIsTailscaleIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.128.0.1", false},
		{"192.168.1.10", false},
		{"fd7a:115c:a1e0::1", true},
		{"fe80::1", false},
	}
	for _, tc := range cases {
		got := isTailscaleIP(net.ParseIP(tc.ip))
		if got != tc.want {
			t.Fatalf("isTailscaleIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}
