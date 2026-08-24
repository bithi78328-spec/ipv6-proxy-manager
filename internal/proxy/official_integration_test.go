package proxy

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ipv6-proxy-manager/internal/model"
)

// This test is enabled in GitHub Actions after installing the pinned official
// 3proxy package. A running process at the timeout proves the generated config
// was accepted and both listeners started.
func TestRenderedConfigStartsOfficial3proxy(t *testing.T) {
	binary := os.Getenv("IP6PM_3PROXY_BINARY")
	if binary == "" {
		t.Skip("official 3proxy binary is not available in this environment")
	}
	config, err := RenderConfig([]model.Proxy{{
		Host: "127.0.0.1", Port: 10080, IPv6: "::1", Username: "user1", Password: "pass1", Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "3proxy.cfg")
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, path)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("3proxy rejected generated config: %v\n%s", err, out)
	}
	if err == nil {
		t.Fatal("3proxy exited immediately instead of serving the generated listeners")
	}
}
