package engine_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"ipv6-proxy-manager/internal/engine"
	"ipv6-proxy-manager/internal/model"
	proxycore "ipv6-proxy-manager/internal/proxy"
)

func TestBuiltInSOCKS5EngineAuthenticatesAndUsesConfiguredIPv6(t *testing.T) {
	target, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback is unavailable: %v", err)
	}
	defer target.Close()
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("::1\n"))
	})}
	go httpServer.Serve(target)
	defer httpServer.Shutdown(context.Background())

	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyPort := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	proxy := model.Proxy{
		Host: "127.0.0.1", Port: proxyPort, IPv6: "::1", Username: "user1", Password: "pass1", Enabled: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engineErr := make(chan error, 1)
	go func() { engineErr <- engine.Run(ctx, []model.Proxy{proxy}) }()

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(proxyPort))
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp4", address, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy listener did not start: %v", dialErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	checker := &proxycore.Checker{
		EndpointHost: "::1", EndpointPort: target.Addr().(*net.TCPAddr).Port, EndpointPath: "/",
		Timeout: 2 * time.Second, Attempts: 1,
	}
	if err := checker.Check(context.Background(), proxy); err != nil {
		t.Fatal(err)
	}
	wrong := proxy
	wrong.Password = "wrong-pass"
	if err := checker.Check(context.Background(), wrong); err == nil {
		t.Fatal("expected invalid credentials to be rejected")
	}
	cancel()
	select {
	case err := <-engineErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal(fmt.Errorf("engine did not stop after cancellation"))
	}
}

func TestTenThousandIdleListeners(t *testing.T) {
	if os.Getenv("IP6PM_TEST_10000") != "1" {
		t.Skip("set IP6PM_TEST_10000=1 for the high-scale listener test")
	}
	proxies := make([]model.Proxy, 10000)
	for i := range proxies {
		proxies[i] = model.Proxy{
			Host: "127.0.0.1", Port: 30000 + i, IPv6: "::1", Username: "user1", Password: "pass1", Enabled: true,
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- engine.Run(ctx, proxies) }()
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp4", "127.0.0.1:39999", 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("10,000th listener did not start: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("10,000-listener engine did not stop")
	}
}
