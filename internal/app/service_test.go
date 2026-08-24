package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ipv6-proxy-manager/internal/model"
	proxycore "ipv6-proxy-manager/internal/proxy"
	"ipv6-proxy-manager/internal/store"
	hostsystem "ipv6-proxy-manager/internal/system"
)

type fakeRunner struct {
	mu          sync.Mutex
	calls       []string
	failRestart bool
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if strings.Contains(call, "ip -j -6 addr show") {
		return []byte(`[{"addr_info":[]}]`), nil
	}
	if strings.Contains(call, "systemctl restart") && f.failRestart {
		f.failRestart = false
		return nil, fmt.Errorf("simulated restart failure")
	}
	return []byte("active"), nil
}

type fakeChecker struct{ err error }

func (f fakeChecker) Check(context.Context, model.Proxy) error { return f.err }

func testService(t *testing.T) (*Service, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	st := store.New(statePath)
	state := model.NewState()
	state.PublicIPv4 = "203.0.113.9"
	state.Interface = "eth0"
	state.IPv6Prefix = "2001:db8:1::/64"
	state.AccessToken = "initial-token"
	if err := st.Save(state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	host := hostsystem.NewHost(runner, "ipv6-proxy-engine.service")
	host.EngineStabilityDelay = 0
	service := &Service{
		Store:   st,
		Host:    host,
		Checker: fakeChecker{},
		Paths: Paths{
			Config:   filepath.Join(dir, "3proxy.cfg"),
			FullList: filepath.Join(dir, "proxies.txt"),
			LiveList: filepath.Join(dir, "proxies-live.txt"),
		},
	}
	return service, runner
}

func TestCreateCheckDisableEnableDelete(t *testing.T) {
	service, _ := testService(t)
	created, report, err := service.Create(context.Background(), proxycore.CreateOptions{
		Count: 2, CredentialMode: "custom", Username: "user1", Password: "pass1",
	})
	if err != nil || !report.Success || len(created) != 2 {
		t.Fatalf("create failed: %+v, %+v, %v", created, report, err)
	}
	if err := service.CheckAll(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	summary, _ := service.Summary()
	if summary.Live != 2 {
		t.Fatalf("expected two live proxies: %+v", summary)
	}
	line := fmt.Sprintf("%s:%d:%s:%s", created[0].Host, created[0].Port, created[0].Username, created[0].Password)
	result, _, err := service.ApplyListAction(context.Background(), "disable", line)
	if err != nil || result.Matched != 1 {
		t.Fatalf("disable failed: %+v %v", result, err)
	}
	summary, _ = service.Summary()
	if summary.Disabled != 1 {
		t.Fatalf("expected one disabled proxy: %+v", summary)
	}
	if _, _, err := service.ApplyListAction(context.Background(), "enable", line); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ApplyListAction(context.Background(), "delete", line); err != nil {
		t.Fatal(err)
	}
	state, _ := service.State()
	if len(state.Proxies) != 1 {
		t.Fatalf("expected one proxy after deletion, got %d", len(state.Proxies))
	}
}

func TestFailedRestartRollsBackConfigAndState(t *testing.T) {
	service, runner := testService(t)
	created, _, err := service.Create(context.Background(), proxycore.CreateOptions{
		Count: 1, CredentialMode: "custom", Username: "user1", Password: "pass1",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(service.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	runner.failRestart = true
	line := fmt.Sprintf("%s:%d:%s:%s", created[0].Host, created[0].Port, created[0].Username, created[0].Password)
	if _, _, err := service.ApplyListAction(context.Background(), "disable", line); err == nil {
		t.Fatal("expected restart failure")
	}
	after, _ := os.ReadFile(service.Paths.Config)
	if string(before) != string(after) {
		t.Fatal("configuration was not rolled back")
	}
	state, _ := service.State()
	if !state.Proxies[0].Enabled {
		t.Fatal("state changed despite failed restart")
	}
}

func TestRotateTokenInvalidatesPreviousURL(t *testing.T) {
	service, _ := testService(t)
	if !service.TokenValid("initial-token") {
		t.Fatal("initial token should be valid")
	}
	newToken, err := service.RotateToken()
	if err != nil {
		t.Fatal(err)
	}
	if service.TokenValid("initial-token") || !service.TokenValid(newToken) || len(newToken) < 40 {
		t.Fatal("token rotation did not invalidate the old URL")
	}
}

func TestBootstrapImportsExistingManualSetup(t *testing.T) {
	service, _ := testService(t)
	dir := filepath.Dir(service.Paths.Config)
	manualList := filepath.Join(dir, "manual-proxies.txt")
	manualConfig := filepath.Join(dir, "manual-3proxy.cfg")
	if err := os.WriteFile(manualList, []byte("203.0.113.9:10000:user1:pass1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manualConfig, []byte("users user1:CL:pass1\nauth strong\nallow user1\nsocks -S65536 -6 -n -a -p10000 -i203.0.113.9 -e2001:db8:1::1\nflush\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service.Paths.ImportList = manualList
	service.Paths.ImportConfigs = []string{manualConfig}
	state, steps, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Proxies) != 1 || state.Proxies[0].IPv6 != "2001:db8:1::1" || len(steps) == 0 {
		t.Fatalf("manual import failed: %+v, steps=%v", state.Proxies, steps)
	}
}
