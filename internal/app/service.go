package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ipv6-proxy-manager/internal/model"
	proxycore "ipv6-proxy-manager/internal/proxy"
	"ipv6-proxy-manager/internal/store"
	hostsystem "ipv6-proxy-manager/internal/system"
)

type Paths struct {
	Config        string
	FullList      string
	LiveList      string
	ImportList    string
	ImportConfigs []string
}

type HealthChecker interface {
	Check(context.Context, model.Proxy) error
}

type Service struct {
	Store   *store.Store
	Host    *hostsystem.Host
	Checker HealthChecker
	Paths   Paths

	mu       sync.Mutex
	checking atomic.Bool
}

type RepairReport struct {
	Success        bool     `json:"success"`
	Steps          []string `json:"steps"`
	AddressesAdded int      `json:"addresses_added"`
	Message        string   `json:"message"`
}

type ActionResult struct {
	Submitted int      `json:"submitted"`
	Matched   int      `json:"matched"`
	NotFound  []string `json:"not_found"`
	Action    string   `json:"action"`
}

func (s *Service) Bootstrap(ctx context.Context) (model.State, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.Store.Load()
	if err != nil {
		return model.State{}, nil, err
	}
	var steps []string
	freshDetection := false
	if state.PublicIPv4 == "" || state.Interface == "" || state.IPv6Prefix == "" {
		detected, err := s.Host.Detect(ctx)
		if err != nil {
			return model.State{}, steps, fmt.Errorf("VPS is unsupported: %w", err)
		}
		state.PublicIPv4 = detected.PublicIPv4
		state.Interface = detected.Interface
		state.IPv6Prefix = detected.IPv6Prefix
		freshDetection = true
		steps = append(steps, fmt.Sprintf("Detected IPv4 %s and routed IPv6 %s on %s", state.PublicIPv4, state.IPv6Prefix, state.Interface))
	}
	if freshDetection {
		if err := s.Host.ProbeRoutedPrefix(ctx, state.Interface, state.IPv6Prefix); err != nil {
			return model.State{}, steps, fmt.Errorf("VPS has IPv6 but does not support a routed multi-address prefix: %w", err)
		}
		steps = append(steps, "Verified that an arbitrary address from the IPv6 prefix reaches the Internet")
	}
	if len(state.Proxies) == 0 {
		count, importSteps, err := s.importExisting(&state)
		if err != nil {
			return model.State{}, steps, fmt.Errorf("import existing proxies: %w", err)
		}
		steps = append(steps, importSteps...)
		if count > 0 {
			steps = append(steps, fmt.Sprintf("Imported %d existing proxies", count))
		}
	}
	if err := s.Store.Save(state); err != nil {
		return model.State{}, steps, err
	}
	if err := s.writeLists(state); err != nil {
		return model.State{}, steps, err
	}
	return state, steps, nil
}

func (s *Service) RotateToken() (string, error) {
	token, err := proxycore.RandomToken()
	if err != nil {
		return "", err
	}
	_, err = s.Store.Update(func(state *model.State) error {
		state.AccessToken = token
		return nil
	})
	return token, err
}

func (s *Service) TokenValid(candidate string) bool {
	state, err := s.Store.Load()
	if err != nil || state.AccessToken == "" || len(candidate) != len(state.AccessToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(state.AccessToken)) == 1
}

func (s *Service) State() (model.State, error) { return s.Store.Load() }

func (s *Service) Create(ctx context.Context, options proxycore.CreateOptions) ([]model.Proxy, RepairReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.Store.Load()
	if err != nil {
		return nil, RepairReport{}, err
	}
	candidate := cloneState(current)
	created, err := proxycore.Allocate(&candidate, options)
	if err != nil {
		return nil, RepairReport{}, err
	}
	report, err := s.applyCandidate(ctx, current, candidate)
	if err != nil {
		return nil, report, err
	}
	return created, report, nil
}

func (s *Service) ApplyListAction(ctx context.Context, action, text string) (ActionResult, RepairReport, error) {
	entries, err := proxycore.ParseList(text)
	if err != nil {
		return ActionResult{}, RepairReport{}, err
	}
	if action != "disable" && action != "enable" && action != "delete" {
		return ActionResult{}, RepairReport{}, fmt.Errorf("action must be disable, enable or delete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.Store.Load()
	if err != nil {
		return ActionResult{}, RepairReport{}, err
	}
	candidate := cloneState(current)
	result := ActionResult{Submitted: len(entries), Action: action}
	matchedIndexes := make(map[int]struct{})
	for _, entry := range entries {
		found := -1
		for i, p := range candidate.Proxies {
			if p.Host == entry.Host && p.Port == entry.Port && p.Username == entry.Username && p.Password == entry.Password {
				found = i
				break
			}
		}
		if found < 0 {
			result.NotFound = append(result.NotFound, fmt.Sprintf("%s:%d:%s:%s", entry.Host, entry.Port, entry.Username, entry.Password))
			continue
		}
		if _, duplicate := matchedIndexes[found]; !duplicate {
			matchedIndexes[found] = struct{}{}
			result.Matched++
		}
	}
	if result.Matched == 0 {
		return result, RepairReport{}, fmt.Errorf("none of the submitted proxies matched this VPS")
	}
	if action == "delete" {
		kept := make([]model.Proxy, 0, len(candidate.Proxies)-result.Matched)
		for i, p := range candidate.Proxies {
			if _, remove := matchedIndexes[i]; !remove {
				kept = append(kept, p)
			}
		}
		candidate.Proxies = kept
	} else {
		for i := range matchedIndexes {
			candidate.Proxies[i].Enabled = action == "enable"
			if candidate.Proxies[i].Enabled {
				candidate.Proxies[i].Status = "pending"
				candidate.Proxies[i].LastError = ""
			} else {
				candidate.Proxies[i].Status = "disabled"
				candidate.Proxies[i].LastError = ""
			}
		}
	}
	report, err := s.applyCandidate(ctx, current, candidate)
	if err != nil {
		return result, report, err
	}
	if action == "delete" {
		remaining := make(map[string]struct{})
		for _, p := range candidate.Proxies {
			remaining[p.IPv6] = struct{}{}
		}
		for _, p := range current.Proxies {
			if _, keep := remaining[p.IPv6]; !keep {
				_ = s.Host.RemoveIPv6(ctx, current.Interface, p.IPv6)
			}
		}
	}
	return result, report, nil
}

func (s *Service) Repair(ctx context.Context) (RepairReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.Store.Load()
	if err != nil {
		return RepairReport{}, err
	}
	return s.applyCandidate(ctx, state, state)
}

// Prepare restores addresses and generated files without controlling systemd.
// It is safe to call from ipv6-proxy-engine.service ExecStartPre during boot.
func (s *Service) Prepare(ctx context.Context) (RepairReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.Store.Load()
	if err != nil {
		return RepairReport{}, err
	}
	report := RepairReport{}
	if state.Interface == "" || state.IPv6Prefix == "" {
		return report, fmt.Errorf("VPS has not been bootstrapped")
	}
	prefix, err := netip.ParsePrefix(state.IPv6Prefix)
	if err != nil {
		return report, fmt.Errorf("stored IPv6 prefix is invalid: %w", err)
	}
	currentAddresses, err := s.Host.CurrentIPv6(ctx, state.Interface)
	if err != nil {
		return report, err
	}
	for _, p := range state.Proxies {
		if !p.Enabled {
			continue
		}
		ip, parseErr := netip.ParseAddr(p.IPv6)
		if parseErr != nil || !prefix.Contains(ip) {
			return report, fmt.Errorf("proxy port %d uses IPv6 outside the routed prefix", p.Port)
		}
		if _, exists := currentAddresses[p.IPv6]; !exists {
			if err := s.Host.AddIPv6(ctx, state.Interface, p.IPv6); err != nil {
				return report, fmt.Errorf("restore IPv6 %s: %w", p.IPv6, err)
			}
			report.AddressesAdded++
		}
	}
	config, err := proxycore.RenderConfig(state.Proxies)
	if err != nil {
		return report, err
	}
	if err := store.WriteAtomic(s.Paths.Config, config, 0o600); err != nil {
		return report, err
	}
	if err := s.writeLists(state); err != nil {
		return report, err
	}
	report.Success = true
	report.Message = "Boot configuration prepared"
	report.Steps = []string{"Restored enabled IPv6 addresses", "Regenerated proxy configuration and text lists"}
	return report, nil
}

func (s *Service) applyCandidate(ctx context.Context, original, candidate model.State) (RepairReport, error) {
	report := RepairReport{}
	if candidate.PublicIPv4 == "" || candidate.Interface == "" || candidate.IPv6Prefix == "" {
		return report, fmt.Errorf("VPS detection is incomplete; run bootstrap first")
	}
	prefix, err := netip.ParsePrefix(candidate.IPv6Prefix)
	if err != nil {
		return report, fmt.Errorf("stored IPv6 prefix is invalid: %w", err)
	}
	config, err := proxycore.RenderConfig(candidate.Proxies)
	if err != nil {
		return report, err
	}
	currentAddresses, err := s.Host.CurrentIPv6(ctx, candidate.Interface)
	if err != nil {
		return report, fmt.Errorf("inspect current IPv6 addresses: %w", err)
	}
	var added []string
	for _, p := range candidate.Proxies {
		if !p.Enabled {
			continue
		}
		ip, parseErr := netip.ParseAddr(p.IPv6)
		if parseErr != nil || !prefix.Contains(ip) {
			return report, fmt.Errorf("proxy port %d uses IPv6 %s outside routed prefix %s", p.Port, p.IPv6, prefix)
		}
		if _, exists := currentAddresses[p.IPv6]; exists {
			continue
		}
		if err := s.Host.AddIPv6(ctx, candidate.Interface, p.IPv6); err != nil {
			for _, address := range added {
				_ = s.Host.RemoveIPv6(ctx, candidate.Interface, address)
			}
			return report, fmt.Errorf("add IPv6 %s: %w", p.IPv6, err)
		}
		added = append(added, p.IPv6)
	}
	report.AddressesAdded = len(added)
	report.Steps = append(report.Steps, fmt.Sprintf("Verified %d enabled IPv6 addresses", enabledCount(candidate.Proxies)))

	oldConfig, oldConfigErr := os.ReadFile(s.Paths.Config)
	rollback := func() {
		if oldConfigErr == nil {
			_ = store.WriteAtomic(s.Paths.Config, oldConfig, 0o600)
			_ = s.Host.RestartEngine(ctx)
		}
		for _, address := range added {
			_ = s.Host.RemoveIPv6(ctx, candidate.Interface, address)
		}
	}
	if err := store.WriteAtomic(s.Paths.Config, config, 0o600); err != nil {
		return report, fmt.Errorf("write 3proxy configuration: %w", err)
	}
	report.Steps = append(report.Steps, "Regenerated the 3proxy configuration")
	if err := s.Host.RestartEngine(ctx); err != nil {
		rollback()
		return report, fmt.Errorf("3proxy failed to restart; previous configuration was restored: %w", err)
	}
	report.Steps = append(report.Steps, "Restarted the proxy engine")
	if err := s.Host.EngineActive(ctx); err != nil {
		rollback()
		return report, fmt.Errorf("proxy engine is not active after restart; previous configuration was restored: %w", err)
	}
	if err := s.Store.Save(candidate); err != nil {
		rollback()
		return report, err
	}
	if err := s.writeLists(candidate); err != nil {
		return report, err
	}
	report.Steps = append(report.Steps, "Regenerated the full proxy text files")
	report.Success = true
	report.Message = "Proxy configuration was rebuilt successfully"
	return report, nil
}

func (s *Service) StartCheck() bool {
	if !s.checking.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer s.checking.Store(false)
		_ = s.CheckAll(context.Background(), 20)
	}()
	return true
}

func (s *Service) CheckAll(ctx context.Context, concurrency int) error {
	if concurrency < 1 {
		concurrency = 1
	}
	s.mu.Lock()
	state, err := s.Store.Update(func(state *model.State) error {
		for i := range state.Proxies {
			if state.Proxies[i].Enabled {
				state.Proxies[i].Status = "checking"
				state.Proxies[i].LastError = ""
			}
		}
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	type result struct {
		index int
		err   error
	}
	jobs := make(chan int)
	results := make(chan result)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results <- result{index: index, err: s.Checker.Check(ctx, state.Proxies[index])}
			}
		}()
	}
	go func() {
		for i, p := range state.Proxies {
			if p.Enabled {
				jobs <- i
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	checks := make(map[int]result)
	for check := range results {
		checks[state.Proxies[check.index].Port] = check
	}
	s.mu.Lock()
	updated, err := s.Store.Update(func(latest *model.State) error {
		for i := range latest.Proxies {
			p := &latest.Proxies[i]
			if !p.Enabled {
				p.Status = "disabled"
				continue
			}
			check, exists := checks[p.Port]
			if !exists {
				continue
			}
			p.LastChecked = time.Now().UTC()
			if check.err == nil {
				p.Status = "live"
				p.LastError = ""
			} else {
				p.Status = "failed"
				p.LastError = check.err.Error()
			}
		}
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.writeLists(updated)
}

func (s *Service) Summary() (model.Summary, error) {
	state, err := s.Store.Load()
	if err != nil {
		return model.Summary{}, err
	}
	summary := model.Summarize(state.Proxies)
	if s.checking.Load() && summary.Checking == 0 {
		summary.Checking = enabledCount(state.Proxies)
	}
	return summary, nil
}

func (s *Service) writeLists(state model.State) error {
	if err := store.WriteAtomic(s.Paths.FullList, proxycore.FormatList(state.Proxies, false), 0o600); err != nil {
		return fmt.Errorf("write full proxy list: %w", err)
	}
	if err := store.WriteAtomic(s.Paths.LiveList, proxycore.FormatList(state.Proxies, true), 0o600); err != nil {
		return fmt.Errorf("write live proxy list: %w", err)
	}
	return nil
}

func (s *Service) importExisting(state *model.State) (int, []string, error) {
	listPath := s.Paths.ImportList
	if listPath == "" {
		listPath = s.Paths.FullList
	}
	listBytes, err := os.ReadFile(listPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	entries, err := proxycore.ParseList(string(listBytes))
	if err != nil {
		return 0, nil, err
	}
	services := make(map[int]proxycore.ImportedService)
	var usedConfig string
	for _, path := range s.Paths.ImportConfigs {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		parsed := proxycore.Parse3ProxyServices(b)
		if len(parsed) > 0 {
			services = parsed
			usedConfig = path
			break
		}
	}
	if len(services) == 0 {
		return 0, nil, fmt.Errorf("found %s but could not find matching 3proxy service mappings", listPath)
	}
	now := time.Now().UTC()
	maxPort := state.NextPort
	for _, entry := range entries {
		service, ok := services[entry.Port]
		if !ok || service.IPv6 == "" {
			continue
		}
		state.Proxies = append(state.Proxies, model.Proxy{
			Host:      entry.Host,
			Port:      entry.Port,
			IPv6:      service.IPv6,
			Username:  entry.Username,
			Password:  entry.Password,
			Enabled:   true,
			Status:    "pending",
			CreatedAt: now,
		})
		if entry.Port >= maxPort {
			maxPort = entry.Port + 1
		}
	}
	state.NextPort = maxPort
	return len(state.Proxies), []string{"Imported configuration from " + filepath.Clean(usedConfig)}, nil
}

func cloneState(state model.State) model.State {
	copy := state
	copy.Proxies = append([]model.Proxy(nil), state.Proxies...)
	return copy
}

func enabledCount(proxies []model.Proxy) int {
	count := 0
	for _, p := range proxies {
		if p.Enabled {
			count++
		}
	}
	return count
}

func FilteredList(state model.State, filter string) []model.Proxy {
	var result []model.Proxy
	for _, p := range state.Proxies {
		include := filter == "all" || filter == ""
		switch filter {
		case "live":
			include = p.Enabled && p.Status == "live"
		case "disabled":
			include = !p.Enabled
		case "failed":
			include = p.Enabled && p.Status == "failed"
		}
		if include {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Port < result[j].Port })
	return result
}

func TextList(proxies []model.Proxy) string {
	return strings.TrimSpace(string(proxycore.FormatList(proxies, false)))
}
