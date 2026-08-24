package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ipv6-proxy-manager/internal/app"
	"ipv6-proxy-manager/internal/model"
	proxycore "ipv6-proxy-manager/internal/proxy"
	"ipv6-proxy-manager/internal/store"
	hostsystem "ipv6-proxy-manager/internal/system"
)

var version = "dev"

func main() {
	statePath := flag.String("state", envOr("IP6PM_STATE", "/var/lib/ipv6-proxy-manager/state.json"), "state file path")
	configPath := flag.String("config", envOr("IP6PM_CONFIG", "/etc/ipv6-proxy-manager/3proxy.cfg"), "generated 3proxy config")
	fullListPath := flag.String("list", envOr("IP6PM_LIST", "/root/proxies.txt"), "full proxy text list")
	liveListPath := flag.String("live-list", envOr("IP6PM_LIVE_LIST", "/root/proxies-live.txt"), "live proxy text list")
	engineService := flag.String("engine-service", envOr("IP6PM_ENGINE_SERVICE", "ipv6-proxy-engine.service"), "systemd proxy engine service")
	listen := flag.String("listen", envOr("IP6PM_LISTEN", "127.0.0.1:8787"), "dashboard listen address")
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
		os.Exit(2)
	}
	service := &app.Service{
		Store:   store.New(*statePath),
		Host:    hostsystem.NewHost(hostsystem.ExecRunner{}, *engineService),
		Checker: proxycore.NewChecker(),
		Paths: app.Paths{
			Config:     *configPath,
			FullList:   *fullListPath,
			LiveList:   *liveListPath,
			ImportList: *fullListPath,
			ImportConfigs: []string{
				"/var/lib/ipv6-proxy-manager/import/3proxy.cfg",
				"/usr/local/3proxy/conf/3proxy.cfg",
				"/etc/3proxy/3proxy.cfg",
				"/etc/3proxy/conf/3proxy.cfg",
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	command := flag.Arg(0)
	switch command {
	case "bootstrap":
		state, steps, err := service.Bootstrap(ctx)
		fatalIf(err)
		printJSON(map[string]any{"public_ipv4": state.PublicIPv4, "interface": state.Interface, "ipv6_prefix": state.IPv6Prefix, "existing_proxies": len(state.Proxies), "steps": steps})
	case "rotate-token":
		token, err := service.RotateToken()
		fatalIf(err)
		fmt.Println(token)
	case "show-url":
		state, err := service.State()
		fatalIf(err)
		if state.PublicIPv4 == "" || state.AccessToken == "" {
			fatalIf(fmt.Errorf("dashboard URL is not ready; run bootstrap and rotate-token"))
		}
		fmt.Printf("https://%s/p/%s/\n", state.PublicIPv4, state.AccessToken)
	case "prepare":
		report, err := service.Prepare(ctx)
		fatalIf(err)
		printJSON(report)
	case "engine":
		fatalIf(runEngine(service, *configPath))
	case "repair":
		report, err := service.Repair(ctx)
		fatalIf(err)
		printJSON(report)
	case "check":
		fatalIf(service.CheckAll(ctx, 20))
		summary, err := service.Summary()
		fatalIf(err)
		printJSON(summary)
	case "serve":
		// A dashboard restart may interrupt an in-flight scan and leave durable
		// statuses at checking. Rechecking on service start also verifies that
		// proxies restored after a VPS reboot are genuinely usable again.
		service.StartCheck()
		server := &http.Server{
			Addr:              *listen,
			Handler:           &app.HTTPServer{Service: service},
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       15 * time.Minute,
			WriteTimeout:      15 * time.Minute,
			IdleTimeout:       60 * time.Second,
		}
		log.Printf("IPv6 Proxy Manager %s listening on %s", version, *listen)
		fatalIf(server.ListenAndServe())
	case "version":
		fmt.Println(version)
	default:
		fatalIf(fmt.Errorf("unknown command %q", command))
	}
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		log.Fatal(err)
	}
}

func fatalIf(err error) {
	if err == nil || err == http.ErrServerClosed {
		return
	}
	log.Fatal(err)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: proxy-manager [flags] bootstrap|rotate-token|show-url|prepare|engine|repair|check|serve|version")
}

const engineWorkerSize = 5000

func runEngine(service *app.Service, configPath string) error {
	state, err := service.State()
	if err != nil {
		return err
	}
	var enabled []model.Proxy
	for _, p := range state.Proxies {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	var chunks [][]model.Proxy
	for start := 0; start < len(enabled); start += engineWorkerSize {
		end := start + engineWorkerSize
		if end > len(enabled) {
			end = len(enabled)
		}
		chunks = append(chunks, enabled[start:end])
	}
	if len(chunks) == 0 {
		chunks = [][]model.Proxy{nil}
	}

	type exitResult struct {
		worker int
		err    error
	}
	exits := make(chan exitResult, len(chunks))
	commands := make([]*exec.Cmd, 0, len(chunks))
	for i, chunk := range chunks {
		config, err := proxycore.RenderEngineConfig(chunk, len(enabled) == 0)
		if err != nil {
			return err
		}
		workerConfig := fmt.Sprintf("%s.worker-%d", configPath, i+1)
		if err := store.WriteAtomic(workerConfig, config, 0o600); err != nil {
			return fmt.Errorf("write engine worker config: %w", err)
		}
		cmd := exec.Command("/bin/3proxy", filepath.Clean(workerConfig))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start 3proxy worker %d: %w", i+1, err)
		}
		commands = append(commands, cmd)
		worker := i + 1
		go func() { exits <- exitResult{worker: worker, err: cmd.Wait()} }()
		if i < len(chunks)-1 {
			select {
			case result := <-exits:
				return fmt.Errorf("3proxy worker %d exited during staged startup: %w", result.worker, result.err)
			case <-time.After(8 * time.Second):
			}
		}
	}
	result := <-exits
	for _, cmd := range commands {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return fmt.Errorf("3proxy worker %d exited: %w", result.worker, result.err)
}
