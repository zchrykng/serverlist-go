package main

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

const (
	addrIsPrivate = iota + 1
	addrIsInvalid
	addrIsInvalidPort
	addrIsUnicode
	addrIsExample
)

var addressErrorHelpTexts = map[int]string{
	addrIsPrivate:     "The server_address you provided is private or local. It is only reachable in your local network.\nIf you meant to host a public server, adjust the setting and make sure your firewall is permitting connections (e.g. port forwarding).",
	addrIsInvalid:     "The server_address you provided is invalid.\nIf you don't have a domain name, try removing the setting from your configuration.",
	addrIsInvalidPort: "The server_address you provided is invalid.\nNote that the value must not include a port number.",
	addrIsUnicode:     "The server_address you provided includes Unicode characters.\nFor domain names you have to use the punycode notation.",
	addrIsExample:     "The server_address you provided is an example value.",
}

type fieldSpec struct {
	required bool
	kind     string
	subKind  string
}

var fields = map[string]fieldSpec{
	"address":           {kind: "string"},
	"port":              {kind: "number"},
	"clients":           {required: true, kind: "number"},
	"clients_max":       {required: true, kind: "number"},
	"uptime":            {required: true, kind: "number"},
	"game_time":         {required: true, kind: "number"},
	"lag":               {kind: "number"},
	"clients_list":      {kind: "list", subKind: "string"},
	"mods":              {kind: "list", subKind: "string"},
	"version":           {required: true, kind: "string"},
	"proto_min":         {required: true, kind: "number"},
	"proto_max":         {required: true, kind: "number"},
	"gameid":            {required: true, kind: "string"},
	"mapgen":            {kind: "string"},
	"url":               {kind: "string"},
	"privs":             {kind: "string"},
	"name":              {required: true, kind: "string"},
	"description":       {required: true, kind: "string"},
	"creative":          {kind: "bool"},
	"dedicated":         {kind: "bool"},
	"damage":            {kind: "bool"},
	"pvp":               {kind: "bool"},
	"password":          {kind: "bool"},
	"rollback":          {kind: "bool"},
	"can_see_far_names": {kind: "bool"},
}

func normalizePort(req map[string]any) error {
	switch v := req["port"].(type) {
	case string:
		port, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		req["port"] = float64(port)
	case float64:
		if v != float64(int(v)) {
			return fmt.Errorf("invalid port type")
		}
	default:
		return fmt.Errorf("invalid port type")
	}
	return nil
}

func checkRequestSchema(req map[string]any) error {
	for name, spec := range fields {
		value, ok := req[name]
		if !ok {
			if spec.required {
				return fmt.Errorf("missing field %s", name)
			}
			continue
		}

		if s, ok := value.(string); ok {
			switch spec.kind {
			case "bool":
				req[name] = strings.EqualFold(s, "true") || s == "1"
				continue
			case "number":
				n, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return err
				}
				req[name] = n
				continue
			}
		}

		switch spec.kind {
		case "string":
			if _, ok := req[name].(string); !ok {
				return fmt.Errorf("field %s must be string", name)
			}
		case "number":
			n, ok := req[name].(float64)
			if !ok {
				return fmt.Errorf("field %s must be number", name)
			}
			if name != "lag" && n != float64(int(n)) {
				return fmt.Errorf("field %s must be integer", name)
			}
		case "bool":
			if _, ok := req[name].(bool); !ok {
				return fmt.Errorf("field %s must be boolean", name)
			}
		case "list":
			items, ok := req[name].([]any)
			if !ok {
				return fmt.Errorf("field %s must be list", name)
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("field %s items must be string", name)
				}
			}
		}
	}
	return nil
}

func checkRequest(req map[string]any) bool {
	for _, field := range []string{"clients", "clients_max", "uptime", "game_time", "lag", "proto_min", "proto_max"} {
		if value, ok := req[field].(float64); ok && value < 0 {
			return false
		}
	}
	if req["proto_min"].(float64) > req["proto_max"].(float64) {
		return false
	}

	const badChars = " \t\v\r\n\x00'"
	if url, ok := req["url"].(string); ok {
		if url == "" || !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) || strings.ContainsAny(url, badChars) {
			delete(req, "url")
		}
	}

	if !sortUniqueStringList(req, "clients_list", badChars) {
		return false
	}
	if clients, ok := req["clients_list"].([]any); ok {
		req["clients"] = float64(len(clients))
	}
	if !sortUniqueStringList(req, "mods", badChars) {
		return false
	}

	for _, field := range []string{"gameid", "mapgen", "version", "privs"} {
		if value, ok := req[field].(string); ok {
			req[field] = strings.Map(func(r rune) rune {
				if strings.ContainsRune(badChars, r) {
					return -1
				}
				return r
			}, value)
		}
	}

	if address, ok := req["address"].(string); !ok || address == "" {
		req["address"] = req["ip"]
	}
	return true
}

func sortUniqueStringList(req map[string]any, field string, badChars string) bool {
	items, ok := req[field].([]any)
	if !ok {
		return true
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := item.(string)
		if value == "" || strings.ContainsAny(value, badChars) {
			return false
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	converted := make([]any, len(out))
	for i, value := range out {
		converted[i] = value
	}
	req[field] = converted
	return true
}

func (a *appState) checkRequestAddress(server *Server) int {
	name := strings.ToLower(server.Address)
	if name == "game.minetest.net" || strings.HasSuffix(name, ".example.com") || strings.HasSuffix(name, ".example.net") || strings.HasSuffix(name, ".example.org") {
		return addrIsExample
	}
	if len(name) > 255 || strings.ContainsAny(name, " @#/*\"'\t\v\r\n\x00") || strings.HasPrefix(name, "-") {
		return addrIsInvalid
	}

	if !isDomain(name) {
		ip := net.ParseIP(name)
		if ip == nil {
			return addrIsInvalid
		}
		if ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return addrIsInvalid
		}
		if a.config.RejectPrivateAddresses && !isPublicIP(ip) {
			return addrIsPrivate
		}
	}

	if a.config.RejectPrivateAddresses {
		if name == "localhost" || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".internal") {
			return addrIsPrivate
		}
	}
	if (strings.Contains(name, ".") && strings.Contains(name, ":")) || (strings.Contains(name, ":") && strings.Contains(name, "[")) {
		return addrIsInvalidPort
	}
	for _, r := range name {
		if r > 127 {
			return addrIsUnicode
		}
	}
	return 0
}

func isDomain(s string) bool {
	i := strings.LastIndex(s, ".")
	return i > 0 && i < len(s)-1 && ((s[i+1] >= 'a' && s[i+1] <= 'z') || (s[i+1] >= 'A' && s[i+1] <= 'Z'))
}

func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified()
}
