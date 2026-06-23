package main

import "testing"

func validRequest() map[string]any {
	return map[string]any{
		"ip":          "203.0.113.20",
		"address":     "server.example.net",
		"port":        float64(30000),
		"clients":     float64(1),
		"clients_max": float64(20),
		"uptime":      float64(12),
		"game_time":   float64(3600),
		"lag":         float64(0.1),
		"version":     "5.9.0",
		"proto_min":   float64(37),
		"proto_max":   float64(44),
		"gameid":      "minetest",
		"name":        "Test Server",
		"description": "A server",
	}
}

func TestCheckRequestSchemaCompatibilityConversions(t *testing.T) {
	req := validRequest()
	req["port"] = float64(30000)
	req["clients"] = "2"
	req["creative"] = "true"

	if err := checkRequestSchema(req); err != nil {
		t.Fatal(err)
	}
	if req["clients"] != float64(2) {
		t.Fatalf("clients = %#v, want converted float64(2)", req["clients"])
	}
	if req["creative"] != true {
		t.Fatalf("creative = %#v, want true", req["creative"])
	}
}

func TestCheckRequestSchemaRejectsFractionalIntegerField(t *testing.T) {
	req := validRequest()
	req["clients"] = 1.5

	if err := checkRequestSchema(req); err == nil {
		t.Fatal("expected fractional clients to fail schema validation")
	}
}

func TestCheckRequestSanitizesAndSorts(t *testing.T) {
	req := validRequest()
	req["clients_list"] = []any{"zach", "anna", "zach"}
	req["mods"] = []any{"default", "bucket", "default"}
	req["url"] = "ftp://example.net"
	req["gameid"] = "mine test\n"

	if err := checkRequestSchema(req); err != nil {
		t.Fatal(err)
	}
	if !checkRequest(req) {
		t.Fatal("checkRequest returned false")
	}

	if _, ok := req["url"]; ok {
		t.Fatal("invalid URL should be removed")
	}
	if req["clients"] != float64(2) {
		t.Fatalf("clients = %#v, want 2 after clients_list dedupe", req["clients"])
	}
	assertList(t, req["clients_list"].([]any), []string{"anna", "zach"})
	assertList(t, req["mods"].([]any), []string{"bucket", "default"})
	if req["gameid"] != "minetest" {
		t.Fatalf("gameid = %q, want sanitized minetest", req["gameid"])
	}
}

func TestCheckRequestDefaultsAddressToRequesterIP(t *testing.T) {
	req := validRequest()
	delete(req, "address")

	if err := checkRequestSchema(req); err != nil {
		t.Fatal(err)
	}
	if !checkRequest(req) {
		t.Fatal("checkRequest returned false")
	}
	if req["address"] != req["ip"] {
		t.Fatalf("address = %#v, want requester IP", req["address"])
	}
}

func TestCheckRequestAddress(t *testing.T) {
	state := &appState{config: &Config{RejectPrivateAddresses: true}}

	tests := []struct {
		name    string
		address string
		want    int
	}{
		{name: "public IPv4", address: "8.8.8.8", want: 0},
		{name: "private IPv4", address: "192.168.1.50", want: addrIsPrivate},
		{name: "reserved local TLD", address: "server.local", want: addrIsPrivate},
		{name: "example host", address: "foo.example.com", want: addrIsExample},
		{name: "unicode", address: "täst.invalid", want: addrIsUnicode},
		{name: "port included", address: "example.net:30000", want: addrIsInvalidPort},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := state.checkRequestAddress(&Server{Address: tc.address})
			if got != tc.want {
				t.Fatalf("checkRequestAddress(%q) = %d, want %d", tc.address, got, tc.want)
			}
		})
	}
}

func assertList(t *testing.T, got []any, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("list len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("list[%d] = %#v, want %q", i, got[i], want[i])
		}
	}
}
