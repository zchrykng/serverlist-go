package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigSCFGAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	})

	config := `
host 0.0.0.0
port 8080
data-dir data
geoip-database geo.mmdb
reject-private-addresses false
allow-update-without-old false
trust-proxy-headers false
log-raw-requests false
banned-ip 198.51.100.9
banned-server Example.Org/30000
purge-time 45m
debug true
`
	if err := os.WriteFile("config.scfg", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SERVERLIST_DATA_DIR", "env-data")
	t.Setenv("SERVERLIST_HOST", "127.0.0.2")
	t.Setenv("SERVERLIST_PORT", "9090")
	t.Setenv("SERVERLIST_REJECT_PRIVATE_ADDRESSES", "yes")
	t.Setenv("SERVERLIST_TRUST_PROXY_HEADERS", "true")
	t.Setenv("SERVERLIST_LOG_RAW_REQUESTS", "true")

	cfg, err := LoadConfig("config.scfg")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "127.0.0.2" || cfg.Port != "9090" {
		t.Fatalf("unexpected listen config: %s:%s", cfg.Host, cfg.Port)
	}
	if want := filepath.Join(cfg.RootPath, "env-data"); cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
	if want := filepath.Join(cfg.RootPath, "geo.mmdb"); cfg.GeoIPDatabase != want {
		t.Fatalf("GeoIPDatabase = %q, want %q", cfg.GeoIPDatabase, want)
	}
	if !cfg.RejectPrivateAddresses {
		t.Fatal("env override should enable RejectPrivateAddresses")
	}
	if cfg.AllowUpdateWithoutOld {
		t.Fatal("allow-update-without-old should be false")
	}
	if !cfg.TrustProxyHeaders {
		t.Fatal("env override should enable TrustProxyHeaders")
	}
	if !cfg.LogRawRequests {
		t.Fatal("env override should enable LogRawRequests")
	}
	if _, ok := cfg.BannedIPs["198.51.100.9"]; !ok {
		t.Fatal("missing banned IP")
	}
	if _, ok := cfg.BannedServers["example.org/30000"]; !ok {
		t.Fatal("banned server should be lower-cased")
	}
	if cfg.PurgeTime != 45*time.Minute {
		t.Fatalf("PurgeTime = %v, want 45m", cfg.PurgeTime)
	}
	if !cfg.Debug {
		t.Fatal("debug should be true")
	}
}

func TestParseDurationSupportsSecondsAndGoDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"600": 10 * time.Minute,
		"3h":  3 * time.Hour,
	}
	for input, want := range tests {
		got, err := parseDuration(input)
		if err != nil {
			t.Fatalf("parseDuration(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseDuration(%q) = %v, want %v", input, got, want)
		}
	}
}
