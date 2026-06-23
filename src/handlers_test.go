package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGeoIPHandlerWithoutDatabase(t *testing.T) {
	state := &appState{}
	req := httptest.NewRequest(http.MethodGet, "/geoip", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	state.geoIPHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=604800" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["continent"] != nil {
		t.Fatalf("continent = %#v, want nil", body["continent"])
	}
}

func TestListJSONHandlerServesPublicFile(t *testing.T) {
	dir := t.TempDir()
	publicPath := filepath.Join(dir, "list.json")
	if err := os.WriteFile(publicPath, []byte(`{"list":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	state := &appState{serverList: &ServerList{PublicPath: publicPath}}
	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	rec := httptest.NewRecorder()

	state.listJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=5" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if rec.Body.String() != `{"list":[]}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestRemoteIPIgnoresProxyHeadersByDefault(t *testing.T) {
	state := &appState{config: &Config{}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.18.0.2:45678"
	req.Header.Set("X-Forwarded-For", "203.0.113.44")

	if got := state.remoteIP(req); got != "172.18.0.2" {
		t.Fatalf("remoteIP = %q, want direct peer", got)
	}
}

func TestRemoteIPUsesTrustedForwardedHeaders(t *testing.T) {
	state := &appState{config: &Config{TrustProxyHeaders: true}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.18.0.2:45678"
	req.Header.Set("X-Forwarded-For", "203.0.113.44, 172.18.0.2")
	req.Header.Set("X-Real-IP", "198.51.100.9")

	if got := state.remoteIP(req); got != "203.0.113.44" {
		t.Fatalf("remoteIP = %q, want first forwarded IP", got)
	}
}

func TestRemoteIPFallsBackToRealIPWhenForwardedForInvalid(t *testing.T) {
	state := &appState{config: &Config{TrustProxyHeaders: true}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.18.0.2:45678"
	req.Header.Set("X-Forwarded-For", "unknown")
	req.Header.Set("X-Real-IP", "198.51.100.9")

	if got := state.remoteIP(req); got != "198.51.100.9" {
		t.Fatalf("remoteIP = %q, want X-Real-IP", got)
	}
}
