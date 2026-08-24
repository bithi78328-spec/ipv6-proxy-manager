package app

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	proxycore "ipv6-proxy-manager/internal/proxy"
	webui "ipv6-proxy-manager/web"
)

type HTTPServer struct{ Service *Service }

func (h *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "p" || !h.Service.TokenValid(parts[1]) {
		http.NotFound(w, r)
		return
	}
	base := "/p/" + parts[1]
	rest := strings.Join(parts[2:], "/")
	if rest == "" {
		h.serveIndex(w, base)
		return
	}
	if strings.HasPrefix(rest, "assets/") {
		h.serveAsset(w, strings.TrimPrefix(rest, "assets/"))
		return
	}
	if strings.HasPrefix(rest, "api/") {
		h.serveAPI(w, r, strings.TrimPrefix(rest, "api/"))
		return
	}
	http.NotFound(w, r)
}

func (h *HTTPServer) serveIndex(w http.ResponseWriter, base string) {
	b, err := fs.ReadFile(webui.Files, "index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(strings.ReplaceAll(string(b), "__BASE__", base)))
}

func (h *HTTPServer) serveAsset(w http.ResponseWriter, name string) {
	if name != "app.js" && name != "style.css" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	b, err := fs.ReadFile(webui.Files, name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	_, _ = w.Write(b)
}

func (h *HTTPServer) serveAPI(w http.ResponseWriter, r *http.Request, endpoint string) {
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") != "application/json" {
		h.writeError(w, http.StatusUnsupportedMediaType, fmt.Errorf("Content-Type must be application/json"))
		return
	}
	switch {
	case endpoint == "summary" && r.Method == http.MethodGet:
		summary, err := h.Service.Summary()
		h.writeJSON(w, summary, err)
	case endpoint == "info" && r.Method == http.MethodGet:
		state, err := h.Service.State()
		info := map[string]any{"public_ipv4": state.PublicIPv4, "interface": state.Interface, "ipv6_prefix": state.IPv6Prefix, "updated_at": state.UpdatedAt}
		h.writeJSON(w, info, err)
	case endpoint == "create" && r.Method == http.MethodPost:
		var request proxycore.CreateOptions
		if err := decodeJSON(r, &request); err != nil {
			h.writeError(w, 400, err)
			return
		}
		created, report, err := h.Service.Create(r.Context(), request)
		if err == nil {
			h.Service.StartCheck()
		}
		h.writeJSON(w, map[string]any{"created": len(created), "repair": report}, err)
	case endpoint == "action" && r.Method == http.MethodPost:
		var request struct {
			Action string `json:"action"`
			List   string `json:"list"`
		}
		if err := decodeJSON(r, &request); err != nil {
			h.writeError(w, 400, err)
			return
		}
		result, report, err := h.Service.ApplyListAction(r.Context(), request.Action, request.List)
		if err == nil && request.Action == "enable" {
			h.Service.StartCheck()
		}
		h.writeJSON(w, map[string]any{"result": result, "repair": report}, err)
	case endpoint == "check" && r.Method == http.MethodPost:
		started := h.Service.StartCheck()
		h.writeJSON(w, map[string]any{"started": started, "message": map[bool]string{true: "Health check started", false: "A health check is already running"}[started]}, nil)
	case endpoint == "repair" && r.Method == http.MethodPost:
		report, err := h.Service.Repair(r.Context())
		if err == nil {
			h.Service.StartCheck()
		}
		h.writeJSON(w, report, err)
	case strings.HasPrefix(endpoint, "list") && r.Method == http.MethodGet:
		h.serveList(w, r, false)
	case strings.HasPrefix(endpoint, "download") && r.Method == http.MethodGet:
		w.Header().Set("Content-Disposition", `attachment; filename="proxies.txt"`)
		h.serveList(w, r, true)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPServer) serveList(w http.ResponseWriter, r *http.Request, _ bool) {
	state, err := h.Service.State()
	if err != nil {
		h.writeError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, TextList(FilteredList(state, r.URL.Query().Get("filter")))+"\n")
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (h *HTTPServer) writeJSON(w http.ResponseWriter, data any, err error) {
	if err != nil {
		h.writeError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *HTTPServer) writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "time": time.Now().UTC()})
}
