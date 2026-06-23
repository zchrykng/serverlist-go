package main

import "testing"

func testServer(ip string, port int, address string, clients int) *Server {
	return &Server{
		IP:      ip,
		Port:    port,
		Address: address,
		Meta: map[string]any{
			"clients":     float64(clients),
			"clients_max": float64(20),
			"game_time":   float64(60 * 60 * 24 * 30),
			"ping":        float64(0.2),
			"proto_min":   float64(37),
			"proto_max":   float64(44),
			"name":        "Server",
		},
		UpdateCount:  1,
		TotalClients: clients,
	}
}

func TestServerTrackUpdatePreservesStartupFields(t *testing.T) {
	old := testServer("203.0.113.10", 30000, "old.example.net", 3)
	old.StartTime = 100
	old.UpdateCount = 2
	old.TotalClients = 8
	old.Meta["clients_top"] = float64(5)
	old.Meta["mods"] = []any{"default"}
	old.Meta["dedicated"] = true

	next := testServer("203.0.113.10", 30000, "new.example.net", 7)
	next.TrackUpdate(old, true)

	if next.StartTime != old.StartTime {
		t.Fatalf("StartTime = %d, want %d", next.StartTime, old.StartTime)
	}
	if next.UpdateCount != 3 {
		t.Fatalf("UpdateCount = %d, want 3", next.UpdateCount)
	}
	if next.TotalClients != 15 {
		t.Fatalf("TotalClients = %d, want 15", next.TotalClients)
	}
	if next.Meta["clients_top"] != float64(7) {
		t.Fatalf("clients_top = %#v, want 7", next.Meta["clients_top"])
	}
	if next.Meta["dedicated"] != true {
		t.Fatal("dedicated startup field was not preserved")
	}
}

func TestServerDuplicatePrefersHigherVerifyLevel(t *testing.T) {
	listed := testServer("203.0.113.10", 30000, "server.example.net", 1)
	listed.VerifyLevel = 3

	candidate := testServer("203.0.113.11", 30000, "SERVER.example.net", 1)
	candidate.VerifyLevel = 1

	if !candidate.IsDuplicate(listed) {
		t.Fatal("candidate with lower verify level should be considered duplicate")
	}
	if listed.IsDuplicate(candidate) {
		t.Fatal("listed server with higher verify level should not be displaced")
	}
}

func TestServerScoreOrdersMoreUsefulServerHigher(t *testing.T) {
	low := testServer("203.0.113.10", 30000, "low.example.net", 1)
	high := testServer("203.0.113.11", 30000, "high.example.net", 8)

	if high.Score() <= low.Score() {
		t.Fatalf("high.Score() = %v, low.Score() = %v", high.Score(), low.Score())
	}
}
