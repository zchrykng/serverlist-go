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
