package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDurationUnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr string
	}{
		{name: "go duration", input: "30s", want: 30 * time.Second},
		{name: "plain seconds", input: "45", want: 45 * time.Second},
		{name: "empty scalar", input: `""`, want: 0},
		{name: "invalid", input: "nope", wantErr: `invalid duration: "nope"`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg struct {
				Timeout Duration `yaml:"timeout"`
			}
			err := yaml.Unmarshal([]byte("timeout: "+tt.input+"\n"), &cfg)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if cfg.Timeout.Duration != tt.want {
				t.Fatalf("duration = %v, want %v", cfg.Timeout.Duration, tt.want)
			}
		})
	}
}

func TestLoadAppliesEnvOverridesAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := strings.TrimSpace(`
server:
  addr: "127.0.0.1:9000"
routing:
  default_upstream: "primary"
upstreams:
  primary:
    type: "cursor"
    base_url: "https://example.com"
  fallback:
    type: "zed"
`)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("ALL2API_ADDR", "0.0.0.0:9999")
	t.Setenv("ALL2API_API_KEYS", "alpha, beta")
	t.Setenv("ALL2API_DEFAULT_UPSTREAM", "fallback")
	t.Setenv("ALL2API_DEBUG", "true")
	t.Setenv("ALL2API_TOOLING_EMULATE_DEBUG", "true")
	t.Setenv("ALL2API_TOOLING_EMULATE_RETRY_ON_REFUSAL", "false")
	t.Setenv("ALL2API_TOOLING_EMULATE_MAX_RETRIES", "5")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Server.Addr != "0.0.0.0:9999" {
		t.Fatalf("server addr = %q", cfg.Server.Addr)
	}
	if len(cfg.Server.APIKeys) != 2 || cfg.Server.APIKeys[0] != "alpha" || cfg.Server.APIKeys[1] != "beta" {
		t.Fatalf("api keys = %#v", cfg.Server.APIKeys)
	}
	if cfg.Routing.DefaultUpstream != "fallback" {
		t.Fatalf("default upstream = %q", cfg.Routing.DefaultUpstream)
	}
	if !cfg.Logging.Debug {
		t.Fatal("expected logging debug to be enabled")
	}
	if !cfg.Tooling.Emulate.Debug {
		t.Fatal("expected tooling emulate debug to be enabled")
	}
	if cfg.Tooling.Emulate.RetryOnRefusal {
		t.Fatal("expected retry_on_refusal override to disable retries")
	}
	if cfg.Tooling.Emulate.MaxRetries != 5 {
		t.Fatalf("max retries = %d", cfg.Tooling.Emulate.MaxRetries)
	}

	fallback := cfg.Upstreams["fallback"]
	if fallback.Timeout.Duration != 120*time.Second {
		t.Fatalf("fallback timeout = %v", fallback.Timeout.Duration)
	}
	if fallback.Auth.Kind != "none" {
		t.Fatalf("fallback auth kind = %q", fallback.Auth.Kind)
	}
}

func TestLoadValidatesCursorBaseURL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := strings.TrimSpace(`
routing:
  default_upstream: "primary"
upstreams:
  primary:
    type: "cursor"
`)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "upstreams.primary.base_url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
