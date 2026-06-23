package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codeberg.org/emersion/go-scfg"
)

type Config struct {
	Host                   string
	Port                   string
	DataDir                string
	GeoIPDatabase          string
	RejectPrivateAddresses bool
	AllowUpdateWithoutOld  bool
	BannedIPs              map[string]struct{}
	BannedServers          map[string]struct{}
	PurgeTime              time.Duration
	Debug                  bool
	RootPath               string
}

func DefaultConfig() *Config {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	return &Config{
		Host:                   "127.0.0.1",
		Port:                   "5000",
		RejectPrivateAddresses: true,
		AllowUpdateWithoutOld:  true,
		BannedIPs:              make(map[string]struct{}),
		BannedServers:          make(map[string]struct{}),
		PurgeTime:              3 * time.Hour,
		RootPath:               root,
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if _, err := os.Stat(path); err == nil {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		var raw struct {
			Host                   string   `scfg:"host"`
			Port                   string   `scfg:"port"`
			DataDir                string   `scfg:"data-dir"`
			GeoIPDatabase          string   `scfg:"geoip-database"`
			RejectPrivateAddresses *bool    `scfg:"reject-private-addresses"`
			AllowUpdateWithoutOld  *bool    `scfg:"allow-update-without-old"`
			BannedIP               []string `scfg:"banned-ip"`
			BannedServer           []string `scfg:"banned-server"`
			PurgeTime              string   `scfg:"purge-time"`
			Debug                  *bool    `scfg:"debug"`
		}
		if err := scfg.NewDecoder(f).Decode(&raw); err != nil {
			return nil, err
		}

		if raw.Host != "" {
			cfg.Host = raw.Host
		}
		if raw.Port != "" {
			cfg.Port = raw.Port
		}
		if raw.DataDir != "" {
			cfg.DataDir = raw.DataDir
		}
		if raw.GeoIPDatabase != "" {
			cfg.GeoIPDatabase = raw.GeoIPDatabase
		}
		if raw.RejectPrivateAddresses != nil {
			cfg.RejectPrivateAddresses = *raw.RejectPrivateAddresses
		}
		if raw.AllowUpdateWithoutOld != nil {
			cfg.AllowUpdateWithoutOld = *raw.AllowUpdateWithoutOld
		}
		if raw.Debug != nil {
			cfg.Debug = *raw.Debug
		}
		if raw.PurgeTime != "" {
			d, err := parseDuration(raw.PurgeTime)
			if err != nil {
				return nil, err
			}
			cfg.PurgeTime = d
		}
		for _, ip := range raw.BannedIP {
			cfg.BannedIPs[ip] = struct{}{}
		}
		for _, server := range raw.BannedServer {
			cfg.BannedServers[strings.ToLower(server)] = struct{}{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	applyEnvOverrides(cfg)
	normalizeConfigPaths(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVERLIST_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("SERVERLIST_PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("SERVERLIST_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("SERVERLIST_GEOIP_DATABASE"); v != "" {
		cfg.GeoIPDatabase = v
	}
	if v, ok := boolFromEnv("SERVERLIST_REJECT_PRIVATE_ADDRESSES"); ok {
		cfg.RejectPrivateAddresses = v
	}
}

func normalizeConfigPaths(cfg *Config) {
	if cfg.DataDir != "" && !filepath.IsAbs(cfg.DataDir) {
		cfg.DataDir = filepath.Join(cfg.RootPath, cfg.DataDir)
	}
	if cfg.GeoIPDatabase != "" && !filepath.IsAbs(cfg.GeoIPDatabase) {
		cfg.GeoIPDatabase = filepath.Join(cfg.RootPath, cfg.GeoIPDatabase)
	}
}

func boolFromEnv(name string) (bool, bool) {
	value := os.Getenv(name)
	if value == "" {
		return false, false
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

func parseDuration(value string) (time.Duration, error) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}
	return time.ParseDuration(value)
}
