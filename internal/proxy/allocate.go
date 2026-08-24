package proxy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/netip"
	"time"

	"ipv6-proxy-manager/internal/model"
)

type CreateOptions struct {
	Count          int    `json:"count"`
	CredentialMode string `json:"credential_mode"`
	Username       string `json:"username"`
	Password       string `json:"password"`
}

func Allocate(state *model.State, options CreateOptions) ([]model.Proxy, error) {
	if options.Count < 1 || options.Count > 5000 {
		return nil, fmt.Errorf("count must be between 1 and 5000")
	}
	if options.CredentialMode != "random" && options.CredentialMode != "custom" {
		return nil, fmt.Errorf("credential mode must be random or custom")
	}
	if options.CredentialMode == "custom" {
		if err := ValidateCredential(options.Username, "username"); err != nil {
			return nil, err
		}
		if err := ValidateCredential(options.Password, "password"); err != nil {
			return nil, err
		}
	}
	prefix, err := netip.ParsePrefix(state.IPv6Prefix)
	if err != nil || !prefix.Addr().Is6() {
		return nil, fmt.Errorf("a valid routed IPv6 prefix has not been detected")
	}
	usedPorts := make(map[int]struct{}, len(state.Proxies))
	usedIPs := make(map[string]struct{}, len(state.Proxies))
	for _, p := range state.Proxies {
		usedPorts[p.Port] = struct{}{}
		usedIPs[p.IPv6] = struct{}{}
	}

	port := state.NextPort
	offset := state.NextIPv6Offset
	created := make([]model.Proxy, 0, options.Count)
	for len(created) < options.Count {
		for port <= 65534 {
			if _, exists := usedPorts[port]; !exists {
				break
			}
			port++
		}
		if port > 65534 {
			return nil, fmt.Errorf("not enough TCP ports remain")
		}

		var ip netip.Addr
		for {
			ip, err = AddOffset(prefix.Masked().Addr(), offset)
			if err != nil || !prefix.Contains(ip) {
				return nil, fmt.Errorf("IPv6 prefix %s does not contain %d available addresses", prefix, options.Count)
			}
			offset++
			if _, exists := usedIPs[ip.String()]; !exists {
				break
			}
		}

		username, password := options.Username, options.Password
		if options.CredentialMode == "random" {
			username, err = randomCredential("u_", 9)
			if err != nil {
				return nil, err
			}
			password, err = randomCredential("p_", 16)
			if err != nil {
				return nil, err
			}
		}
		created = append(created, model.Proxy{
			Host:      state.PublicIPv4,
			Port:      port,
			IPv6:      ip.String(),
			Username:  username,
			Password:  password,
			Enabled:   true,
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		})
		usedPorts[port] = struct{}{}
		usedIPs[ip.String()] = struct{}{}
		port++
	}
	state.NextPort = port
	state.NextIPv6Offset = offset
	state.Proxies = append(state.Proxies, created...)
	return created, nil
}

func AddOffset(base netip.Addr, offset uint64) (netip.Addr, error) {
	if !base.Is6() {
		return netip.Addr{}, fmt.Errorf("base address is not IPv6")
	}
	b := base.As16()
	carry := offset
	for i := len(b) - 1; i >= 0 && carry > 0; i-- {
		sum := uint64(b[i]) + (carry & 0xff)
		b[i] = byte(sum)
		carry = (carry >> 8) + (sum >> 8)
	}
	if carry != 0 {
		return netip.Addr{}, fmt.Errorf("IPv6 address overflow")
	}
	return netip.AddrFrom16(b), nil
}

func randomCredential(prefix string, bytesCount int) (string, error) {
	b := make([]byte, bytesCount)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secure credentials: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func RandomToken() (string, error) { return randomCredential("", 32) }
