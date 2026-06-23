package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type Server struct {
	IP           string         `json:"ip"`
	Port         int            `json:"port"`
	Address      string         `json:"address"`
	Meta         map[string]any `json:"meta"`
	StartTime    int64          `json:"startTime"`
	UpdateCount  int            `json:"updateCount"`
	UpdateTime   int64          `json:"updateTime"`
	TotalClients int            `json:"totalClients"`
	VerifyLevel  int            `json:"verifyLevel"`
}

func ServerFromRequest(data map[string]any) *Server {
	meta := make(map[string]any, len(data))
	for k, v := range data {
		meta[k] = v
	}

	ip := meta["ip"].(string)
	port := int(meta["port"].(float64))
	address := meta["address"].(string)
	delete(meta, "ip")
	delete(meta, "port")
	delete(meta, "address")
	delete(meta, "action")

	return &Server{IP: ip, Port: port, Address: address, Meta: meta}
}

func (s *Server) ToListJSON() map[string]any {
	out := make(map[string]any, len(s.Meta)+3)
	for k, v := range s.Meta {
		out[k] = v
	}
	out["address"] = s.Address
	out["port"] = s.Port
	out["pop_v"] = s.AverageClients()
	return out
}

func (s *Server) String() string {
	out := fmt.Sprintf("%s/%d %q %q", s.IP, s.Port, s.Address, s.Meta["name"])
	if clients := int(number(s.Meta["clients"])); clients > 0 {
		out += fmt.Sprintf(" (%dP)", clients)
	}
	return out
}

func (s *Server) ErrorPK() string {
	return fmt.Sprintf("%s/%d/%s", s.IP, s.Port, s.Address)
}

func (s *Server) AverageClients() int {
	if s.UpdateCount == 0 {
		return 0
	}
	return int(math.Round(float64(s.TotalClients) / float64(s.UpdateCount)))
}

func (s *Server) Score() float64 {
	meta := s.Meta
	points := number(meta["clients"])

	capacity := int(number(meta["clients_max"]) * 0.80)
	clients := int(number(meta["clients"]))
	if clients > capacity {
		points -= float64(clients - capacity)
	}

	points += math.Min(8, number(meta["game_time"])/(60*60*24*30))
	points += math.Min(4, float64(s.AverageClients())/2)

	if number(meta["clients_max"]) > 200 {
		points -= 8
	}
	if number(meta["ping"]) > 0.4 {
		points -= (number(meta["ping"]) - 0.4) * 8
	}
	if number(meta["proto_min"]) <= 32 && number(meta["proto_max"]) > 36 {
		points *= 0.6
	}
	return points
}

func (s *Server) TrackUpdate(old *Server, isUpdate bool) {
	now := time.Now().Unix()
	if old != nil {
		s.StartTime = old.StartTime
	} else {
		s.StartTime = now
	}
	s.UpdateTime = now

	if isUpdate {
		for _, field := range []string{"dedicated", "rollback", "mapgen", "privs", "can_see_far_names", "mods"} {
			if value, ok := old.Meta[field]; ok {
				s.Meta[field] = value
			}
		}
	} else {
		s.Meta["uptime"] = float64(0)
	}

	clients := int(number(s.Meta["clients"]))
	if old != nil {
		s.UpdateCount = old.UpdateCount + 1
		s.TotalClients = old.TotalClients + clients
		top := int(number(old.Meta["clients_top"]))
		if clients > top {
			top = clients
		}
		s.Meta["clients_top"] = float64(top)
	} else {
		s.UpdateCount = 1
		s.TotalClients = clients
		s.Meta["clients_top"] = float64(clients)
	}
}

func (s *Server) IsDuplicate(other *Server) bool {
	if s.Port == other.Port && strings.EqualFold(s.Address, other.Address) {
		if s.IP == other.IP {
			return false
		}
		return s.VerifyLevel < other.VerifyLevel
	}
	return false
}

func number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
