package proxy

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"ipv6-proxy-manager/internal/model"
)

func TestAllocateContinuesAndSkipsUsed(t *testing.T) {
	state := model.NewState()
	state.PublicIPv4 = "203.0.113.9"
	state.IPv6Prefix = "2001:db8:1234::/64"
	state.NextPort = 10000
	state.Proxies = []model.Proxy{{Port: 10000, IPv6: "2001:db8:1234::1"}}
	created, err := Allocate(&state, CreateOptions{Count: 2, CredentialMode: "custom", Username: "user1", Password: "pass1"})
	if err != nil {
		t.Fatal(err)
	}
	if created[0].Port != 10001 || created[0].IPv6 != "2001:db8:1234::2" || created[1].Port != 10002 {
		t.Fatalf("unexpected allocation: %+v", created)
	}
}

func TestAllocateRejectsInsufficientPrefix(t *testing.T) {
	state := model.NewState()
	state.PublicIPv4 = "203.0.113.9"
	state.IPv6Prefix = "2001:db8::/127"
	_, err := Allocate(&state, CreateOptions{Count: 2, CredentialMode: "custom", Username: "user1", Password: "pass1"})
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("expected capacity error, got %v", err)
	}
}

func TestAddOffsetCarries(t *testing.T) {
	base := netip.MustParseAddr("2001:db8::ffff")
	got, err := AddOffset(base, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "2001:db8::1:1" {
		t.Fatalf("unexpected address %s", got)
	}
}

func TestRenderConfigAndListParsing(t *testing.T) {
	proxies := []model.Proxy{
		{Host: "203.0.113.9", Port: 10001, IPv6: "2001:db8::2", Username: "user2", Password: "pass2", Enabled: false},
		{Host: "203.0.113.9", Port: 10000, IPv6: "2001:db8::1", Username: "user1", Password: "pass1", Enabled: true},
	}
	config, err := RenderConfig(proxies)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	if !strings.Contains(text, "socks -S65536 -6 -n -a -p10000 -i203.0.113.9 -e2001:db8::1") || strings.Contains(text, "-p10001") {
		t.Fatalf("unexpected config:\n%s", text)
	}
	entries, err := ParseList(string(FormatList(proxies, false)))
	if err != nil || len(entries) != 2 || entries[0].Port != 10000 {
		t.Fatalf("unexpected list parse: %+v, %v", entries, err)
	}
}

func TestRenderConfigKeepsLargeCredentialSetsOnSafeLines(t *testing.T) {
	proxies := make([]model.Proxy, 5000)
	for i := range proxies {
		proxies[i] = model.Proxy{
			Host: "203.0.113.9", Port: 10000 + i, IPv6: fmt.Sprintf("2001:db8::%x", i+1),
			Username: fmt.Sprintf("user_%04d", i), Password: fmt.Sprintf("password_%04d", i), Enabled: true,
		}
	}
	config, err := RenderConfig(proxies)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, line := range strings.Split(string(config), "\n") {
		if len(line) > 1024 {
			t.Fatalf("generated an unsafe %d-byte configuration line", len(line))
		}
		if strings.HasPrefix(line, "users ") {
			users++
		}
	}
	if users != len(proxies) {
		t.Fatalf("expected %d users commands, got %d", len(proxies), users)
	}
}

func TestParseExisting3ProxyServices(t *testing.T) {
	services := Parse3ProxyServices([]byte("allow user\nsocks -S65536 -6 -n -a -p10000 -i203.0.113.9 -e2001:db8::1\nflush\n"))
	if got := services[10000]; got.Host != "203.0.113.9" || got.IPv6 != "2001:db8::1" {
		t.Fatalf("unexpected imported service: %+v", got)
	}
}
