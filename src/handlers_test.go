package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestLogRawAnnounceRequestRestoresBody(t *testing.T) {
	var logs bytes.Buffer
	state := &appState{
		config: &Config{LogRawRequests: true},
		logger: log.New(&logs, "", 0),
	}
	req := httptest.NewRequest(http.MethodPost, "/announce", strings.NewReader("json=%7B%7D"))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err := state.logRawAnnounceRequest(req, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "json=%7B%7D" {
		t.Fatalf("restored body = %q", body)
	}
	if !strings.Contains(logs.String(), `raw_body="json=%7B%7D"`) {
		t.Fatalf("raw log missing body: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `Content-Type=["application/x-www-form-urlencoded"]`) {
		t.Fatalf("raw log missing content type header: %s", logs.String())
	}
}

func TestParseAnnounceFormSupportsMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("json", `{"action":"delete","address":"luanti.king.fyi","port":30000}`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/announce", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := parseAnnounceForm(req); err != nil {
		t.Fatal(err)
	}
	if got := req.FormValue("json"); got != `{"action":"delete","address":"luanti.king.fyi","port":30000}` {
		t.Fatalf("json form value = %q", got)
	}
}

func TestParseAnnounceFormSupportsURLEncoded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/announce", strings.NewReader("json=%7B%7D"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err := parseAnnounceForm(req); err != nil {
		t.Fatal(err)
	}
	if got := req.FormValue("json"); got != `{}` {
		t.Fatalf("json form value = %q", got)
	}
}
