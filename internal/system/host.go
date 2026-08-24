package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"sort"
	"strings"
)

type Detection struct {
	PublicIPv4 string `json:"public_ipv4"`
	Interface  string `json:"interface"`
	IPv6Prefix string `json:"ipv6_prefix"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Supported  bool   `json:"supported"`
	Reason     string `json:"reason,omitempty"`
}

type Host struct {
	Runner        Runner
	EngineService string
}

func NewHost(r Runner, engineService string) *Host {
	return &Host{Runner: r, EngineService: engineService}
}

type route struct {
	Dst     string `json:"dst"`
	Dev     string `json:"dev"`
	PrefSrc string `json:"prefsrc"`
	Src     string `json:"src"`
	Scope   string `json:"scope"`
}

func (h *Host) Detect(ctx context.Context) (Detection, error) {
	result := Detection{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if runtime.GOOS != "linux" {
		result.Reason = "the proxy host must run Linux"
		return result, errors.New(result.Reason)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		result.Reason = "only amd64 and arm64 VPS architectures are supported"
		return result, errors.New(result.Reason)
	}

	output, err := h.Runner.Run(ctx, "ip", "-j", "-4", "route", "get", "1.1.1.1")
	if err != nil {
		return result, fmt.Errorf("detect public IPv4 route: %w", err)
	}
	v4Output := output
	output, err = h.Runner.Run(ctx, "ip", "-j", "-6", "route", "show")
	if err != nil {
		return result, fmt.Errorf("detect IPv6 routes: %w", err)
	}
	publicIPv4, iface, prefix, err := detectRoutes(v4Output, output)
	if err != nil {
		result.Reason = err.Error()
		return result, err
	}
	result.PublicIPv4 = publicIPv4
	result.Interface = iface
	result.IPv6Prefix = prefix
	result.Supported = true
	return result, nil
}

func detectRoutes(v4Output, v6Output []byte) (publicIPv4, iface, ipv6Prefix string, err error) {
	var v4routes []route
	if parseErr := json.Unmarshal(v4Output, &v4routes); parseErr != nil || len(v4routes) == 0 {
		return "", "", "", fmt.Errorf("cannot parse the IPv4 route: %w", parseErr)
	}
	publicIPv4 = v4routes[0].PrefSrc
	if publicIPv4 == "" {
		publicIPv4 = v4routes[0].Src
	}
	if ip, parseErr := netip.ParseAddr(publicIPv4); parseErr != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
		return "", "", "", errors.New("no public IPv4 source address was found")
	}
	var v6routes []route
	if parseErr := json.Unmarshal(v6Output, &v6routes); parseErr != nil {
		return "", "", "", fmt.Errorf("cannot parse IPv6 routes: %w", parseErr)
	}
	type candidate struct {
		prefix netip.Prefix
		dev    string
	}
	var candidates []candidate
	for _, r := range v6routes {
		if r.Dst == "" || r.Dst == "default" || r.Dev == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(r.Dst)
		if err != nil || !prefix.Addr().Is6() || !prefix.Addr().IsGlobalUnicast() || prefix.Addr().IsPrivate() {
			continue
		}
		candidates = append(candidates, candidate{prefix: prefix.Masked(), dev: r.Dev})
	}
	if len(candidates) == 0 {
		return "", "", "", errors.New("no routed global IPv6 prefix was found; a single /128 address is not enough")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].prefix.Bits() < candidates[j].prefix.Bits() })
	chosen := candidates[0]
	return publicIPv4, chosen.dev, chosen.prefix.String(), nil
}

func (h *Host) CurrentIPv6(ctx context.Context, iface string) (map[string]struct{}, error) {
	out, err := h.Runner.Run(ctx, "ip", "-j", "-6", "addr", "show", "dev", iface)
	if err != nil {
		return nil, err
	}
	var links []struct {
		AddrInfo []struct {
			Local string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, fmt.Errorf("parse IPv6 addresses: %w", err)
	}
	result := make(map[string]struct{})
	for _, link := range links {
		for _, info := range link.AddrInfo {
			if ip, err := netip.ParseAddr(info.Local); err == nil && ip.Is6() {
				result[ip.String()] = struct{}{}
			}
		}
	}
	return result, nil
}

func (h *Host) AddIPv6(ctx context.Context, iface, address string) error {
	_, err := h.Runner.Run(ctx, "ip", "-6", "addr", "add", address+"/128", "dev", iface, "nodad")
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "file exists") {
		return err
	}
	return nil
}

// ProbeRoutedPrefix proves that an arbitrary address from the detected prefix
// can be used for outbound Internet traffic. Merely seeing a /64 route is not
// sufficient: some providers expose a route but only permit one assigned /128.
func (h *Host) ProbeRoutedPrefix(ctx context.Context, iface, prefixText string) error {
	prefix, err := netip.ParsePrefix(prefixText)
	if err != nil || !prefix.Addr().Is6() {
		return fmt.Errorf("invalid IPv6 prefix %q", prefixText)
	}
	current, err := h.CurrentIPv6(ctx, iface)
	if err != nil {
		return fmt.Errorf("read current IPv6 addresses: %w", err)
	}
	candidate := prefix.Masked().Addr()
	for attempts := 0; attempts < 4096; attempts++ {
		candidate = candidate.Next()
		if !candidate.IsValid() || !prefix.Contains(candidate) {
			return fmt.Errorf("IPv6 prefix %s has no free test address", prefix)
		}
		if _, exists := current[candidate.String()]; !exists {
			break
		}
	}
	if err := h.AddIPv6(ctx, iface, candidate.String()); err != nil {
		return fmt.Errorf("add temporary IPv6 test address: %w", err)
	}
	defer h.RemoveIPv6(context.Background(), iface, candidate.String())
	endpoints := []string{"https://api64.ipify.org", "https://ifconfig.co/ip"}
	var failures []string
	for _, endpoint := range endpoints {
		out, runErr := h.Runner.Run(ctx, "curl", "-6", "--interface", candidate.String(), "--silent", "--show-error", "--fail", "--max-time", "15", endpoint)
		if runErr != nil {
			failures = append(failures, runErr.Error())
			continue
		}
		got, parseErr := netip.ParseAddr(strings.TrimSpace(string(out)))
		if parseErr == nil && got == candidate {
			return nil
		}
		failures = append(failures, fmt.Sprintf("%s returned %q", endpoint, strings.TrimSpace(string(out))))
	}
	return fmt.Errorf("the provider did not route a temporary address from %s: %s", prefix, strings.Join(failures, "; "))
}

func (h *Host) RemoveIPv6(ctx context.Context, iface, address string) error {
	_, err := h.Runner.Run(ctx, "ip", "-6", "addr", "del", address+"/128", "dev", iface)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "cannot assign requested address") {
		return err
	}
	return nil
}

func (h *Host) RestartEngine(ctx context.Context) error {
	if h.EngineService == "" {
		return errors.New("engine service name is empty")
	}
	_, err := h.Runner.Run(ctx, "systemctl", "restart", h.EngineService)
	return err
}

func (h *Host) EngineActive(ctx context.Context) error {
	_, err := h.Runner.Run(ctx, "systemctl", "is-active", "--quiet", h.EngineService)
	return err
}
