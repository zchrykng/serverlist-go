package main

import (
	"log"
	"net"
	"path/filepath"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

type GeoIP struct {
	reader *maxminddb.Reader
	logger *log.Logger
}

func OpenGeoIP(cfg *Config, logger *log.Logger) (*GeoIP, error) {
	paths := make([]string, 0, 2)
	if cfg.GeoIPDatabase != "" {
		paths = append(paths, cfg.GeoIPDatabase)
	} else {
		if matches, _ := filepath.Glob(filepath.Join(cfg.RootPath, "dbip-country-lite-*.mmdb")); len(matches) > 0 {
			paths = append(paths, matches...)
		}
		if cfg.DataDir != "" {
			if matches, _ := filepath.Glob(filepath.Join(cfg.DataDir, "dbip-country-lite-*.mmdb")); len(matches) > 0 {
				paths = append(paths, matches...)
			}
		}
	}

	if len(paths) == 0 {
		logger.Print("For working GeoIP download the database from https://db-ip.com/db/download/ip-to-country-lite and place the .mmdb file in the app root or data folder.")
		return nil, nil
	}

	reader, err := maxminddb.Open(paths[0])
	if err != nil {
		return nil, err
	}
	return &GeoIP{reader: reader, logger: logger}, nil
}

func (g *GeoIP) Close() error {
	return g.reader.Close()
}

func (g *GeoIP) LookupContinent(ipString string) string {
	ipString = strings.TrimPrefix(ipString, "::ffff:")
	ip := net.ParseIP(ipString)
	if ip == nil {
		return ""
	}

	var record struct {
		Continent struct {
			Code string `maxminddb:"code"`
		} `maxminddb:"continent"`
	}
	if err := g.reader.Lookup(ip, &record); err != nil {
		g.logger.Printf("Unable to get GeoIP continent data for %s: %v", ipString, err)
		return ""
	}
	if record.Continent.Code == "" {
		g.logger.Printf("Unable to get GeoIP continent data for %s.", ipString)
	}
	return record.Continent.Code
}
