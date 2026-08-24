package engine

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"ipv6-proxy-manager/internal/model"
)

const (
	handshakeTimeout = 20 * time.Second
	maxConnections   = 16
)

type listener struct {
	proxy model.Proxy
	net.Listener
}

// Run serves every enabled proxy with Go's network poller. Unlike 3proxy's
// service-per-thread model, thousands of idle listeners do not consume one OS
// thread each, which keeps 10,000 ports viable on small VPS instances.
func Run(ctx context.Context, proxies []model.Proxy) error {
	var active []model.Proxy
	for _, p := range proxies {
		if p.Enabled {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		keepalive, err := net.Listen("tcp4", "127.0.0.1:65535")
		if err != nil {
			return fmt.Errorf("open keepalive listener: %w", err)
		}
		<-ctx.Done()
		_ = keepalive.Close()
		return nil
	}

	listeners := make([]listener, 0, len(active))
	for _, p := range active {
		ln, err := net.Listen("tcp4", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
		if err != nil {
			closeListeners(listeners)
			return fmt.Errorf("listen on proxy port %d: %w", p.Port, err)
		}
		listeners = append(listeners, listener{proxy: p, Listener: ln})
	}

	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for _, item := range listeners {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := acceptLoop(ctx, item); err != nil {
				select {
				case errCh <- fmt.Errorf("proxy port %d: %w", item.proxy.Port, err):
				default:
				}
			}
		}()
	}

	var result error
	select {
	case <-ctx.Done():
	case result = <-errCh:
	}
	closeListeners(listeners)
	wg.Wait()
	return result
}

func acceptLoop(ctx context.Context, item listener) error {
	connections := make(chan struct{}, maxConnections)
	for {
		conn, err := item.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}
		select {
		case connections <- struct{}{}:
			go func() {
				defer func() { <-connections }()
				serveConnection(ctx, conn, item.proxy)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func serveConnection(parent context.Context, client net.Conn, proxy model.Proxy) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := authenticate(client, proxy.Username, proxy.Password); err != nil {
		return
	}
	target, reply, err := readConnectRequest(client)
	if err != nil {
		writeReply(client, reply)
		return
	}
	dialCtx, cancel := context.WithTimeout(parent, handshakeTimeout)
	defer cancel()
	dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(proxy.IPv6)}}
	upstream, err := dialer.DialContext(dialCtx, "tcp", target)
	if err != nil {
		writeReply(client, 0x05)
		return
	}
	defer upstream.Close()
	if err := writeReply(client, 0x00); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	relay(client, upstream)
}

func authenticate(conn net.Conn, username, password string) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 0x05 || header[1] == 0 {
		return fmt.Errorf("invalid SOCKS greeting")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	found := false
	for _, method := range methods {
		found = found || method == 0x02
	}
	if !found {
		_, _ = conn.Write([]byte{0x05, 0xff})
		return fmt.Errorf("username/password authentication was not offered")
	}
	if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
		return err
	}
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 0x01 {
		return fmt.Errorf("invalid authentication request")
	}
	user := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, user); err != nil {
		return err
	}
	length := []byte{0}
	if _, err := io.ReadFull(conn, length); err != nil {
		return err
	}
	pass := make([]byte, int(length[0]))
	if _, err := io.ReadFull(conn, pass); err != nil {
		return err
	}
	valid := len(user) == len(username) && len(pass) == len(password)
	if valid {
		valid = subtle.ConstantTimeCompare(user, []byte(username)) == 1 && subtle.ConstantTimeCompare(pass, []byte(password)) == 1
	}
	if !valid {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return fmt.Errorf("invalid credentials")
	}
	_, err := conn.Write([]byte{0x01, 0x00})
	return err
}

func readConnectRequest(conn net.Conn) (string, byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", 0x01, err
	}
	if header[0] != 0x05 || header[1] != 0x01 || header[2] != 0x00 {
		return "", 0x07, fmt.Errorf("only SOCKS5 CONNECT is supported")
	}
	var host string
	switch header[3] {
	case 0x01:
		b := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", 0x01, err
		}
		host = net.IP(b).String()
	case 0x03:
		length := []byte{0}
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", 0x01, err
		}
		b := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", 0x01, err
		}
		host = string(b)
	case 0x04:
		b := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", 0x01, err
		}
		host = net.IP(b).String()
	default:
		return "", 0x08, fmt.Errorf("unsupported address type")
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", 0x01, err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), 0x00, nil
}

func writeReply(conn net.Conn, code byte) error {
	_, err := conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}

func relay(client, upstream net.Conn) {
	done := make(chan struct{}, 2)
	copyOne := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyOne(upstream, client)
	go copyOne(client, upstream)
	<-done
	<-done
}

func closeListeners(listeners []listener) {
	for _, item := range listeners {
		_ = item.Close()
	}
}
