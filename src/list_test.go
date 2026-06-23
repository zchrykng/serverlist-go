package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerListSavePublicJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.RootPath = dir
	cfg.DataDir = dir

	sl, err := NewServerList(cfg)
	if err != nil {
		t.Fatal(err)
	}

	low := testServer("203.0.113.10", 30000, "low.example.net", 1)
	high := testServer("203.0.113.11", 30000, "high.example.net", 8)
	sl.Update(low)
	sl.Update(high)

	if err := sl.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "list.json"))
	if err != nil {
		t.Fatal(err)
	}
	var public struct {
		Total struct {
			Servers int `json:"servers"`
			Clients int `json:"clients"`
		} `json:"total"`
		TotalMax struct {
			Servers int `json:"servers"`
			Clients int `json:"clients"`
		} `json:"total_max"`
		List []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
			PopV    int    `json:"pop_v"`
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}

	if public.Total.Servers != 2 || public.Total.Clients != 9 {
		t.Fatalf("total = %+v, want 2 servers and 9 clients", public.Total)
	}
	if public.TotalMax.Servers != 2 || public.TotalMax.Clients != 9 {
		t.Fatalf("total_max = %+v, want 2 servers and 9 clients", public.TotalMax)
	}
	if len(public.List) != 2 {
		t.Fatalf("list length = %d, want 2", len(public.List))
	}
	if public.List[0].Address != "high.example.net" {
		t.Fatalf("first server = %q, want high score first", public.List[0].Address)
	}
	if public.List[0].Port != 30000 || public.List[0].PopV != 8 {
		t.Fatalf("first server fields = %+v", public.List[0])
	}
}

func TestServerListLoadsLegacyStorage(t *testing.T) {
	dir := t.TempDir()
	storage := `{
		"list": [
			["203.0.113.10", 30000, "server.example.net", {"clients": 4, "clients_max": 20, "game_time": 3600, "ping": 0.2, "proto_min": 37, "proto_max": 44, "name": "Legacy"}, {"startTime": 10, "updateCount": 2, "updateTime": 20, "totalClients": 8, "verifyLevel": 3}]
		],
		"maxServers": 3,
		"maxClients": 12
	}`
	if err := os.WriteFile(filepath.Join(dir, "store.json"), []byte(storage), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.RootPath = dir
	cfg.DataDir = dir
	sl, err := NewServerList(cfg)
	if err != nil {
		t.Fatal(err)
	}

	server := sl.Get("203.0.113.10", 30000)
	if server == nil {
		t.Fatal("legacy server was not loaded")
	}
	if server.Address != "server.example.net" || server.VerifyLevel != 3 {
		t.Fatalf("loaded server = %+v", server)
	}
	if sl.MaxServers != 3 || sl.MaxClients != 12 {
		t.Fatalf("max values = %d/%d, want 3/12", sl.MaxServers, sl.MaxClients)
	}
}

func TestServerListPurgeOld(t *testing.T) {
	sl := &ServerList{}
	old := testServer("203.0.113.10", 30000, "old.example.net", 1)
	old.UpdateTime = time.Now().Add(-2 * time.Hour).Unix()
	fresh := testServer("203.0.113.11", 30000, "fresh.example.net", 1)
	fresh.UpdateTime = time.Now().Unix()
	sl.List = []*Server{old, fresh}

	sl.PurgeOld(time.Now().Add(-time.Hour))

	if len(sl.List) != 1 || sl.List[0] != fresh {
		t.Fatalf("remaining list = %#v, want only fresh server", sl.List)
	}
}
