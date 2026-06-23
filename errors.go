package main

import (
	"sync"
	"time"
)

type ErrorInfo struct {
	Warn bool
	Text string
}

type errorEntry struct {
	expires time.Time
	info    ErrorInfo
}

type ErrorTracker struct {
	mu    sync.Mutex
	table map[string]errorEntry
}

func NewErrorTracker() *ErrorTracker {
	return &ErrorTracker{table: make(map[string]errorEntry)}
}

func (et *ErrorTracker) Put(key string, info ErrorInfo) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.table[key] = errorEntry{expires: time.Now().Add(10 * time.Minute), info: info}
}

func (et *ErrorTracker) Remove(key string) {
	et.mu.Lock()
	defer et.mu.Unlock()
	delete(et.table, key)
}

func (et *ErrorTracker) Get(key string) *ErrorInfo {
	et.mu.Lock()
	defer et.mu.Unlock()
	entry, ok := et.table[key]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	info := entry.info
	return &info
}

func (et *ErrorTracker) Cleanup() {
	et.mu.Lock()
	defer et.mu.Unlock()
	now := time.Now()
	for key, entry := range et.table {
		if now.After(entry.expires) {
			delete(et.table, key)
		}
	}
}
