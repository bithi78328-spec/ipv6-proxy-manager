package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"ipv6-proxy-manager/internal/model"
)

type Checker struct {
	EndpointHost string
	EndpointPort int
	EndpointPath string
	FallbackHost string
	FallbackPort int
	FallbackPath string
	Timeout      time.Duration
}

func NewChecker() *Checker {
	return &Checker{
		EndpointHost: "api64.ipify.org",
		EndpointPort: 80,
		EndpointPath: "/",
		FallbackHost: "ifconfig.co",
		FallbackPort: 80,
		FallbackPath: "/ip",
		Timeout:      12 * time.Second,
	}
}

func (c *Checker) Check(ctx context.Context, p model.Proxy) error {
	err := c.checkEndpoint(ctx, p, c.EndpointHost, c.EndpointPort, c.EndpointPath)
	if err == nil || c.FallbackHost == "" {
		return err
	}
	fallbackErr := c.checkEndpoint(ctx, p, c.FallbackHost, c.FallbackPort, c.FallbackPath)
	if fallbackErr == nil {
		return nil
	}
	return fmt.Errorf("both health endpoints failed: primary: %v; fallback: %v", err, fallbackErr)
}

func (c *Checker) checkEndpoint(ctx context.Context, p model.Proxy, endpointHost string, endpointPort int, endpointPath string) error {
	if !p.Enabled {
		return fmt.Errorf("proxy is disabled")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	if err != nil {
		return fmt.Errorf("port is not reachable: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := socksAuthenticate(conn, p.Username, p.Password); err != nil {
		return err
	}
	if err := socksConnect(conn, endpointHost, endpointPort); err != nil {
		return err
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\nUser-Agent: ipv6-proxy-manager\r\n\r\n", endpointPath, endpointHost)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		return fmt.Errorf("read egress test response: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("egress test returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return fmt.Errorf("read egress address: %w", err)
	}
	got, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil {
		return fmt.Errorf("egress test did not return an IP address")
	}
	want, err := netip.ParseAddr(p.IPv6)
	if err != nil || got != want {
		return fmt.Errorf("outbound IPv6 mismatch: expected %s, received %s", p.IPv6, got)
	}
	return nil
}

func socksAuthenticate(conn net.Conn, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return fmt.Errorf("SOCKS credentials are too long")
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return fmt.Errorf("send SOCKS greeting: %w", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("read SOCKS greeting: %w", err)
	}
	if reply[0] != 0x05 || reply[1] != 0x02 {
		return fmt.Errorf("SOCKS server did not accept username/password authentication")
	}
	auth := []byte{0x01, byte(len(username))}
	auth = append(auth, username...)
	auth = append(auth, byte(len(password)))
	auth = append(auth, password...)
	if _, err := conn.Write(auth); err != nil {
		return fmt.Errorf("send SOCKS credentials: %w", err)
	}
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("read SOCKS authentication result: %w", err)
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("SOCKS username or password was rejected")
	}
	return nil
}

func socksConnect(conn net.Conn, host string, port int) error {
	if len(host) > 255 {
		return fmt.Errorf("health-check hostname is too long")
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	request = append(request, host...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)
	if _, err := conn.Write(request); err != nil {
		return fmt.Errorf("send SOCKS connect request: %w", err)
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read SOCKS connect result: %w", err)
	}
	if header[0] != 0x05 || header[1] != 0x00 {
		return fmt.Errorf("SOCKS outbound connection failed with code %d", header[1])
	}
	var addressLength int
	switch header[3] {
	case 0x01:
		addressLength = 4
	case 0x04:
		addressLength = 16
	case 0x03:
		length := []byte{0}
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		addressLength = int(length[0])
	default:
		return fmt.Errorf("SOCKS server returned an unknown address type")
	}
	if _, err := io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
		return fmt.Errorf("read SOCKS bound address: %w", err)
	}
	return nil
}
