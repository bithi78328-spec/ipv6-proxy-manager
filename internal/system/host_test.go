package system

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type probeRunner struct{ fail bool }

func (p probeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	if strings.Contains(call, "ip -j -6 addr show") {
		return []byte(`[{"addr_info":[{"local":"2001:db8:1234::1"}]}]`), nil
	}
	if name == "curl" {
		if p.fail {
			return nil, fmt.Errorf("no IPv6 route")
		}
		return []byte("2001:db8:1234::2\n"), nil
	}
	return nil, nil
}

func TestDetectRoutesChoosesRoutedPrefix(t *testing.T) {
	v4 := []byte(`[{"dst":"1.1.1.1","dev":"eth0","prefsrc":"203.0.113.7"}]`)
	v6 := []byte(`[
      {"dst":"default","dev":"eth0"},
      {"dst":"fe80::/64","dev":"eth0"},
      {"dst":"2001:db8:1234::/64","dev":"eth0"}
    ]`)
	ipv4, iface, prefix, err := detectRoutes(v4, v6)
	if err != nil {
		t.Fatal(err)
	}
	if ipv4 != "203.0.113.7" || iface != "eth0" || prefix != "2001:db8:1234::/64" {
		t.Fatalf("unexpected detection: %s %s %s", ipv4, iface, prefix)
	}
}

func TestDetectRoutesRejectsSingleAddress(t *testing.T) {
	v4 := []byte(`[{"dev":"eth0","prefsrc":"203.0.113.7"}]`)
	v6 := []byte(`[{"dst":"default","dev":"eth0"}]`)
	_, _, _, err := detectRoutes(v4, v6)
	if err == nil || !strings.Contains(err.Error(), "no routed global IPv6 prefix") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProbeRoutedPrefix(t *testing.T) {
	host := NewHost(probeRunner{}, "engine.service")
	if err := host.ProbeRoutedPrefix(context.Background(), "eth0", "2001:db8:1234::/64"); err != nil {
		t.Fatal(err)
	}
	host = NewHost(probeRunner{fail: true}, "engine.service")
	if err := host.ProbeRoutedPrefix(context.Background(), "eth0", "2001:db8:1234::/64"); err == nil {
		t.Fatal("expected routed-prefix probe failure")
	}
}
