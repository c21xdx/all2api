package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfigFlagDefaultAndOverrides(t *testing.T) {
	cfg, provided, rest, err := parseConfigFlag([]string{"login", "zed"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg != defaultConfigPath {
		t.Fatalf("cfg = %q", cfg)
	}
	if provided {
		t.Fatal("expected config not to be marked provided")
	}
	if len(rest) != 2 || rest[0] != "login" || rest[1] != "zed" {
		t.Fatalf("rest = %#v", rest)
	}

	t.Setenv("ALL2API_CONFIG", "from-env.yaml")
	cfg, provided, _, err = parseConfigFlag([]string{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg != "from-env.yaml" {
		t.Fatalf("cfg = %q", cfg)
	}
	if provided {
		t.Fatal("expected env config not to be marked provided")
	}
}

func TestParseConfigFlagRecognizesConfigFlag(t *testing.T) {
	cfg, provided, rest, err := parseConfigFlag([]string{"-config", "a.yaml", "login", "tabbit"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg != "a.yaml" {
		t.Fatalf("cfg = %q", cfg)
	}
	if !provided {
		t.Fatal("expected config to be marked provided")
	}
	if len(rest) != 2 || rest[0] != "login" || rest[1] != "tabbit" {
		t.Fatalf("rest = %#v", rest)
	}
}

func TestResolveConfigPathPrefersDockerMountedConfig(t *testing.T) {
	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Create ./config/config.yaml
	if err := os.MkdirAll(filepath.Join(wd, "config"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wd, "config", "config.yaml"), []byte("server: {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := resolveConfigPath("config.yaml", false)
	if got != filepath.Join("config", "config.yaml") {
		t.Fatalf("resolved = %q", got)
	}

	// Explicit flag should not be overridden.
	got = resolveConfigPath("explicit.yaml", true)
	if got != "explicit.yaml" {
		t.Fatalf("resolved = %q", got)
	}
}
