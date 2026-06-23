package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
)

func (a *appState) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("/list", a.listJSON)
	mux.HandleFunc("/geoip", a.geoIPHandler)
	mux.HandleFunc("/announce", a.announce)
	return mux
}

func (a *appState) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func (a *appState) listJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(listSaveInterval.Seconds())))
	http.ServeFile(w, r, a.serverList.PublicPath)
}

func (a *appState) geoIPHandler(w http.ResponseWriter, r *http.Request) {
	continent := ""
	if a.geoIP != nil {
		continent = a.geoIP.LookupContinent(a.remoteIP(r))
	}

	w.Header().Set("Cache-Control", "private, max-age=604800")
	w.Header().Set("Content-Type", "application/json")

	resp := map[string]any{"continent": nil}
	if continent != "" {
		resp["continent"] = continent
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *appState) announce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		a.reject(w, r, http.StatusMethodNotAllowed, "", 0, "method_not_allowed", "Method not allowed.")
		return
	}

	ip := a.remoteIP(r)
	if err := a.logRawAnnounceRequest(r, ip); err != nil {
		a.logger.Printf("raw announce logging failed: client_ip=%s peer=%s error=%v", ip, r.RemoteAddr, err)
	}
	if _, banned := a.config.BannedIPs[ip]; banned {
		a.reject(w, r, http.StatusForbidden, "", 0, "banned_ip", "Banned (IP).")
		return
	}

	if err := r.ParseForm(); err != nil {
		a.reject(w, r, http.StatusBadRequest, "", 0, "form_parse_error", "Unable to process form data.")
		return
	}
	jsonData := r.FormValue("json")
	if jsonData == "" {
		a.reject(w, r, http.StatusBadRequest, "", 0, "missing_json", "Missing JSON data.")
		return
	}
	if len(jsonData) > 11*1024 {
		a.reject(w, r, http.StatusRequestEntityTooLarge, "", 0, "json_too_large", fmt.Sprintf("JSON data is too big (%d).", len(jsonData)))
		return
	}

	var req map[string]any
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		a.reject(w, r, http.StatusBadRequest, "", 0, "json_decode_error", "Unable to process JSON data.")
		return
	}

	action, _ := req["action"].(string)
	delete(req, "action")
	if action != "start" && action != "update" && action != "delete" {
		a.reject(w, r, http.StatusBadRequest, action, 0, "invalid_action", "Invalid action field.")
		return
	}

	req["ip"] = ip
	if _, ok := req["port"]; !ok {
		req["port"] = float64(30000)
	}
	if err := normalizePort(req); err != nil {
		a.reject(w, r, http.StatusBadRequest, action, 0, "invalid_port", "JSON data does not conform to schema.")
		return
	}

	port := int(req["port"].(float64))
	if a.isServerBanned(ip, port, req) {
		a.reject(w, r, http.StatusForbidden, action, port, "banned_server", "Banned (Server).")
		return
	}

	old := a.serverList.Get(ip, port)
	if action == "delete" {
		if old == nil {
			a.logger.Printf("announce delete ignored: client_ip=%s peer=%s port=%d reason=not_found", ip, r.RemoteAddr, port)
			writeText(w, http.StatusOK, "Server to remove not found.")
			return
		}
		a.serverList.Remove(old)
		a.logChangedServer(old, nil)
		writeText(w, http.StatusOK, "Removed from server list.")
		return
	}

	if err := checkRequestSchema(req); err != nil {
		a.reject(w, r, http.StatusBadRequest, action, port, "schema_error", "JSON data does not conform to schema.")
		return
	}
	if !checkRequest(req) {
		a.reject(w, r, http.StatusBadRequest, action, port, "invalid_request", "Incorrect JSON data.")
		return
	}

	uptime := int(req["uptime"].(float64))
	if action == "update" && old == nil {
		if a.config.AllowUpdateWithoutOld && uptime > 0 {
			a.logger.Printf("announce update promoted to start: client_ip=%s peer=%s port=%d uptime=%d", ip, r.RemoteAddr, port, uptime)
			action = "start"
		} else {
			a.logger.Printf("announce update ignored: client_ip=%s peer=%s port=%d reason=old_server_not_found", ip, r.RemoteAddr, port)
			writeText(w, http.StatusOK, "Server to update not found.")
			return
		}
	}

	server := ServerFromRequest(req)
	if action == "start" || old.Address != server.Address {
		if errCode := a.checkRequestAddress(server); errCode != 0 {
			a.reject(w, r, http.StatusBadRequest, action, port, "address_"+addressErrorCodeName(errCode), addressErrorHelpTexts[errCode])
			return
		}
	}
	if action == "update" && int(old.Meta["uptime"].(float64)) > int(server.Meta["uptime"].(float64)) {
		a.reject(w, r, http.StatusBadRequest, action, port, "non_monotonic_uptime", "Detected non-monotonic uptime.")
		return
	}

	server.TrackUpdate(old, action == "update")
	errInfo := a.errorTracker.Get(server.ErrorPK())
	go a.finishRequest(server)
	a.logger.Printf("announce accepted: action=%s client_ip=%s peer=%s address=%s port=%d name=%q clients=%d uptime=%d verify_pending=true",
		action, ip, r.RemoteAddr, server.Address, server.Port, server.Meta["name"], int(number(server.Meta["clients"])), int(number(server.Meta["uptime"])))

	if errInfo != nil {
		prefix := "Request has been filed, but the previous one encountered an error:\n"
		if errInfo.Warn {
			prefix = "Request has been filed, but there is a warning from the previous one:\n"
		}
		a.logger.Printf("announce accepted with previous verification issue: client_ip=%s address=%s port=%d warn=%t issue=%q",
			ip, server.Address, server.Port, errInfo.Warn, errInfo.Text)
		writeText(w, http.StatusConflict, prefix+errInfo.Text)
		return
	}
	writeText(w, http.StatusAccepted, "Request has been filed.")
}

func (a *appState) isServerBanned(ip string, port int, req map[string]any) bool {
	if _, ok := a.config.BannedServers[fmt.Sprintf("%s/%d", ip, port)]; ok {
		return true
	}
	address, _ := req["address"].(string)
	address = strings.ToLower(address)
	if address == "" {
		return false
	}
	if _, ok := a.config.BannedServers[fmt.Sprintf("%s/%d", address, port)]; ok {
		return true
	}
	_, ok := a.config.BannedServers[address]
	return ok
}

func (a *appState) reject(w http.ResponseWriter, r *http.Request, status int, action string, port int, reason string, text string) {
	clientIP := ""
	if r != nil {
		clientIP = a.remoteIP(r)
	}
	a.logger.Printf("request rejected: status=%d reason=%s method=%s path=%s client_ip=%s peer=%s action=%s port=%d response=%q",
		status, reason, r.Method, r.URL.Path, clientIP, r.RemoteAddr, action, port, firstLine(text))
	http.Error(w, text, status)
}

func (a *appState) logRawAnnounceRequest(r *http.Request, clientIP string) error {
	if a.config == nil || !a.config.LogRawRequests {
		return nil
	}

	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
		if err != nil {
			return err
		}
		if err := r.Body.Close(); err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	truncated := false
	if len(body) > 64*1024 {
		body = body[:64*1024]
		truncated = true
	}

	a.logger.Printf("raw announce request: method=%s path=%s client_ip=%s peer=%s content_type=%q content_length=%d transfer_encoding=%q headers=%s body_len=%d body_truncated=%t raw_body=%q",
		r.Method, r.URL.RequestURI(), clientIP, r.RemoteAddr, r.Header.Get("Content-Type"), r.ContentLength,
		strings.Join(r.TransferEncoding, ","), formatHeadersForLog(r.Header), len(body), truncated, string(body))
	return nil
}

func formatHeadersForLog(headers http.Header) string {
	if len(headers) == 0 {
		return "{}"
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		lower := strings.ToLower(name)
		if lower == "authorization" || lower == "cookie" || lower == "set-cookie" {
			parts = append(parts, fmt.Sprintf("%s=<redacted>", name))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%q", name, headers.Values(name)))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func addressErrorCodeName(code int) string {
	switch code {
	case addrIsPrivate:
		return "private"
	case addrIsInvalid:
		return "invalid"
	case addrIsInvalidPort:
		return "invalid_port"
	case addrIsUnicode:
		return "unicode"
	case addrIsExample:
		return "example"
	default:
		return "unknown"
	}
}

func (a *appState) remoteIP(r *http.Request) string {
	if a.config != nil && a.config.TrustProxyHeaders {
		if ip := forwardedIP(r); ip != "" {
			return ip
		}
	}
	return directRemoteIP(r.RemoteAddr)
}

func forwardedIP(r *http.Request) string {
	if value := r.Header.Get("X-Forwarded-For"); value != "" {
		for _, part := range strings.Split(value, ",") {
			if ip := normalizeIPString(strings.TrimSpace(part)); ip != "" {
				return ip
			}
		}
	}
	if value := r.Header.Get("X-Real-IP"); value != "" {
		return normalizeIPString(strings.TrimSpace(value))
	}
	return ""
}

func directRemoteIP(remoteAddr string) string {
	return normalizeIPString(remoteAddr)
}

func normalizeIPString(value string) string {
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "::ffff:")
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimPrefix(host, "::ffff:")
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}
