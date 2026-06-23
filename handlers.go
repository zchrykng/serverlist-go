package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
		continent = a.geoIP.LookupContinent(remoteIP(r))
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
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	ip := remoteIP(r)
	if _, banned := a.config.BannedIPs[ip]; banned {
		http.Error(w, "Banned (IP).", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to process form data.", http.StatusBadRequest)
		return
	}
	jsonData := r.FormValue("json")
	if jsonData == "" {
		http.Error(w, "Missing JSON data.", http.StatusBadRequest)
		return
	}
	if len(jsonData) > 11*1024 {
		http.Error(w, fmt.Sprintf("JSON data is too big (%d).", len(jsonData)), http.StatusRequestEntityTooLarge)
		return
	}

	var req map[string]any
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		http.Error(w, "Unable to process JSON data.", http.StatusBadRequest)
		return
	}

	action, _ := req["action"].(string)
	delete(req, "action")
	if action != "start" && action != "update" && action != "delete" {
		http.Error(w, "Invalid action field.", http.StatusBadRequest)
		return
	}

	req["ip"] = ip
	if _, ok := req["port"]; !ok {
		req["port"] = float64(30000)
	}
	if err := normalizePort(req); err != nil {
		http.Error(w, "JSON data does not conform to schema.", http.StatusBadRequest)
		return
	}

	port := int(req["port"].(float64))
	if a.isServerBanned(ip, port, req) {
		http.Error(w, "Banned (Server).", http.StatusForbidden)
		return
	}

	old := a.serverList.Get(ip, port)
	if action == "delete" {
		if old == nil {
			writeText(w, http.StatusOK, "Server to remove not found.")
			return
		}
		a.serverList.Remove(old)
		a.logChangedServer(old, nil)
		writeText(w, http.StatusOK, "Removed from server list.")
		return
	}

	if err := checkRequestSchema(req); err != nil {
		http.Error(w, "JSON data does not conform to schema.", http.StatusBadRequest)
		return
	}
	if !checkRequest(req) {
		http.Error(w, "Incorrect JSON data.", http.StatusBadRequest)
		return
	}

	uptime := int(req["uptime"].(float64))
	if action == "update" && old == nil {
		if a.config.AllowUpdateWithoutOld && uptime > 0 {
			action = "start"
		} else {
			writeText(w, http.StatusOK, "Server to update not found.")
			return
		}
	}

	server := ServerFromRequest(req)
	if action == "start" || old.Address != server.Address {
		if errCode := a.checkRequestAddress(server); errCode != 0 {
			http.Error(w, addressErrorHelpTexts[errCode], http.StatusBadRequest)
			return
		}
	}
	if action == "update" && int(old.Meta["uptime"].(float64)) > int(server.Meta["uptime"].(float64)) {
		http.Error(w, "Detected non-monotonic uptime.", http.StatusBadRequest)
		return
	}

	server.TrackUpdate(old, action == "update")
	errInfo := a.errorTracker.Get(server.ErrorPK())
	go a.finishRequest(server)

	if errInfo != nil {
		prefix := "Request has been filed, but the previous one encountered an error:\n"
		if errInfo.Warn {
			prefix = "Request has been filed, but there is a warning from the previous one:\n"
		}
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

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return strings.TrimPrefix(host, "::ffff:")
}

func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}
