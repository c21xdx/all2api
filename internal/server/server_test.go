package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lhpqaq/all2api/internal/config"
)

func newTestServer(apiKeys []string) *Server {
	s := &Server{
		cfg: config.Config{Server: config.ServerConfig{APIKeys: apiKeys}},
		mux: http.NewServeMux(),
	}
	s.mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return s
}

func TestRouterWithoutAPIKeysPassesRequests(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	newTestServer(nil).Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "pong" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestRouterHealthBypassesAPIKeyCheck(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	newTestServer([]string{"secret"}).Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestRouterAcceptsAuthorizationAndXAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "authorization bearer", header: "Authorization", value: "Bearer secret"},
		{name: "x-api-key", header: "X-API-Key", value: "secret"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.Header.Set(tt.header, tt.value)

			newTestServer([]string{"secret"}).Router().ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
		})
	}
}

func TestRouterRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer wrong")

	newTestServer([]string{"secret"}).Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Unauthorized") {
		t.Fatalf("body = %q", string(body))
	}
}
