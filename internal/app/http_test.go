package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecretDashboardRouting(t *testing.T) {
	service, _ := testService(t)
	handler := &HTTPServer{Service: service}

	request := httptest.NewRequest(http.MethodGet, "/p/wrong-token/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wrong token returned %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/p/initial-token/", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "IPv6 Proxy Manager") {
		t.Fatalf("dashboard failed: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/p/initial-token/api/summary", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":0`) {
		t.Fatalf("summary failed: %d %s", response.Code, response.Body.String())
	}
}
