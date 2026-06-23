package main

import (
	"context"
	"fmt"
	"net"
	"strings"
)

func (a *appState) finishRequest(server *Server) {
	a.errorTracker.Remove(server.ErrorPK())
	a.logger.Printf("verification started: client_ip=%s address=%s port=%d name=%q", server.IP, server.Address, server.Port, server.Meta["name"])

	addrs, err := net.DefaultResolver.LookupIPAddr(context.Background(), server.Address)
	if err != nil {
		text := fmt.Sprintf("Unable to get address info for %s", server.Address)
		a.logger.Printf("%s (IP: %s)", text, server.IP)
		a.errorTracker.Put(server.ErrorPK(), ErrorInfo{Text: text})
		return
	}

	if server.IP == server.Address {
		server.VerifyLevel = 3
	} else {
		addressSet := make(map[string]struct{}, len(addrs))
		haveV4 := false
		haveV6 := false
		for _, addr := range addrs {
			ip := addr.IP.String()
			addressSet[ip] = struct{}{}
			if addr.IP.To4() != nil {
				haveV4 = true
			} else {
				haveV6 = true
			}
		}

		if _, ok := addressSet[server.IP]; ok {
			server.VerifyLevel = 3
		} else if (strings.Contains(server.IP, ":") && !haveV6) || (strings.Contains(server.IP, ".") && !haveV4) {
			server.VerifyLevel = 2
			a.logger.Printf("verification tolerated IP family mismatch: client_ip=%s address=%s port=%d have_ipv4=%t have_ipv6=%t verify_level=%d",
				server.IP, server.Address, server.Port, haveV4, haveV6, server.VerifyLevel)
		} else {
			text := fmt.Sprintf("Requester IP %s does not match host %s", server.IP, server.Address)
			if isDomain(server.Address) {
				valid := make([]string, 0, len(addrs))
				for _, addr := range addrs {
					valid = append(valid, addr.IP.String())
				}
				text += " (valid: " + strings.Join(valid, " ") + ")"
			}
			a.logger.Print(text)
			a.errorTracker.Put(server.ErrorPK(), ErrorInfo{Warn: true, Text: text})
			server.VerifyLevel = 1
		}
	}
	a.logger.Printf("verification DNS complete: client_ip=%s address=%s port=%d resolved=%s verify_level=%d",
		server.IP, server.Address, server.Port, resolvedIPs(addrs), server.VerifyLevel)

	if a.serverList.CheckDuplicate(server) {
		text := fmt.Sprintf("Server %s port %d already exists on the list", server.Address, server.Port)
		a.logger.Printf("%s (IP: %s)", text, server.IP)
		a.errorTracker.Put(server.ErrorPK(), ErrorInfo{Text: text})
		return
	}

	if a.geoIP != nil && len(addrs) > 0 {
		if continent := a.geoIP.LookupContinent(addrs[len(addrs)-1].IP.String()); continent != "" {
			server.Meta["geo_continent"] = continent
			a.logger.Printf("verification GeoIP complete: address=%s port=%d continent=%s", server.Address, server.Port, continent)
		}
	}

	ping, ok, err := ServerUp(server.Address, server.Port)
	if err != nil {
		a.logger.Printf("unexpected exception during serverUp: %v", err)
	}
	if !ok {
		text := fmt.Sprintf("Server %s port %d did not respond to ping", server.Address, server.Port)
		if isDomain(server.Address) && len(addrs) > 0 {
			text += fmt.Sprintf(" (tried %s)", addrs[0].IP.String())
		}
		a.logger.Print(text)
		a.errorTracker.Put(server.ErrorPK(), ErrorInfo{Text: text})
		return
	}
	server.Meta["ping"] = float64(int(ping.Seconds()*100000+0.5)) / 100000
	a.logger.Printf("verification ping complete: address=%s port=%d ping=%.5f", server.Address, server.Port, number(server.Meta["ping"]))

	old := a.serverList.Update(server)
	a.logChangedServer(old, server)
	a.logger.Printf("verification complete: client_ip=%s address=%s port=%d verify_level=%d listed=true",
		server.IP, server.Address, server.Port, server.VerifyLevel)
}

func (a *appState) logChangedServer(old *Server, newServer *Server) {
	if old == nil {
		a.logger.Printf("New server added: %s", newServer)
		return
	}
	if newServer == nil {
		a.logger.Printf("Server was deleted: %s", old)
		return
	}
	if old.Address != newServer.Address || old.Meta["name"] != newServer.Meta["name"] {
		a.logger.Printf("Server changed: %s", newServer)
	}
	if abs(int(number(old.Meta["clients"]))-int(number(newServer.Meta["clients"]))) >= 9 {
		a.logger.Printf("Player movement: %s (old: %dP)", newServer, int(number(old.Meta["clients"])))
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func resolvedIPs(addrs []net.IPAddr) string {
	values := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		values = append(values, addr.IP.String())
	}
	return strings.Join(values, ",")
}
