package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

const listSaveInterval = 5 * time.Second

type appState struct {
	config       *Config
	geoIP        *GeoIP
	serverList   *ServerList
	errorTracker *ErrorTracker
	logger       *log.Logger
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	cfg, err := LoadConfig("config.scfg")
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	geoIP, err := OpenGeoIP(cfg, logger)
	if err != nil {
		logger.Fatalf("failed to open GeoIP database: %v", err)
	}
	if geoIP != nil {
		defer geoIP.Close()
	}

	serverList, err := NewServerList(cfg)
	if err != nil {
		logger.Fatalf("failed to initialize server list: %v", err)
	}
	if err := serverList.Save(); err != nil {
		logger.Fatalf("failed to save initial server list: %v", err)
	}

	state := &appState{
		config:       cfg,
		geoIP:        geoIP,
		serverList:   serverList,
		errorTracker: NewErrorTracker(),
		logger:       logger,
	}

	go state.runTimer()

	addr := cfg.Host + ":" + cfg.Port
	logger.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, state.routes()); err != nil {
		logger.Fatal(err)
	}
}

func (a *appState) runTimer() {
	ticker := time.NewTicker(listSaveInterval)
	defer ticker.Stop()

	nextCleanup := time.Now()
	for range ticker.C {
		if !time.Now().Before(nextCleanup) {
			a.serverList.PurgeOld(time.Now().Add(-a.config.PurgeTime))
			a.errorTracker.Cleanup()
			nextCleanup = time.Now().Add(time.Minute)
		}
		if err := a.serverList.Save(); err != nil {
			a.logger.Printf("failed to save server list: %v", err)
		}
	}
}
