package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ipv6-proxy-manager/internal/app"
	"ipv6-proxy-manager/internal/engine"
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
	command := flag.Arg(0)
	checker := proxycore.NewChecker()
	if command == "serve" {
		var closeChecker func()
		var err error
		checker, closeChecker, err = proxycore.NewLoopbackChecker()
		fatalIf(err)
		defer closeChecker()
	}
	service := &app.Service{
		Store:   store.New(*statePath),
		Host:    hostsystem.NewHost(hostsystem.ExecRunner{}, *engineService),
		Checker: checker,
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
		state, err := service.State()
		fatalIf(err)
		engineCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fatalIf(engine.Run(engineCtx, state.Proxies))
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
		// statuses at checking. After a VPS reboot, systemd considers the engine
		// started before all thousands of listeners have finished opening. Wait
		// for a stable engine PID before scanning so that boot timing cannot turn
		// healthy proxies into false failures.
		startupCtx, stopStartupCheck := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stopStartupCheck()
		go func() {
			if err := service.Host.EngineActive(startupCtx); err != nil {
				log.Printf("startup health check skipped: proxy engine was not ready: %v", err)
				return
			}
			service.StartCheck()
		}()
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

