package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type ServerList struct {
	List        []*Server `json:"list"`
	MaxServers  int       `json:"maxServers"`
	MaxClients  int       `json:"maxClients"`
	StoragePath string    `json:"-"`
	PublicPath  string    `json:"-"`

	mu       sync.RWMutex
	modified bool
	debug    bool
}

func NewServerList(cfg *Config) (*ServerList, error) {
	base := cfg.RootPath
	publicDir := cfg.RootPath
	if cfg.DataDir != "" {
		base = cfg.DataDir
		publicDir = cfg.DataDir
	}

	sl := &ServerList{
		StoragePath: filepath.Join(base, "store.json"),
		PublicPath:  filepath.Join(publicDir, "list.json"),
		modified:    true,
		debug:       cfg.Debug,
	}
	if err := os.MkdirAll(filepath.Dir(sl.StoragePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(sl.PublicPath), 0o755); err != nil {
		return nil, err
	}
	return sl, sl.Load()
}

func (sl *ServerList) Get(ip string, port int) *Server {
	_, server := sl.getWithIndex(ip, port)
	return server
}

func (sl *ServerList) getWithIndex(ip string, port int) (int, *Server) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	for i, server := range sl.List {
		if server.IP == ip && server.Port == port {
			return i, server
		}
	}
	return -1, nil
}

func (sl *ServerList) CheckDuplicate(other *Server) bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	for _, server := range sl.List {
		if other.IsDuplicate(server) {
			return true
		}
	}
	return false
}

func (sl *ServerList) Remove(server *Server) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for i, candidate := range sl.List {
		if candidate == server {
			sl.List = append(sl.List[:i], sl.List[i+1:]...)
			sl.modified = true
			return
		}
	}
}

func (sl *ServerList) PurgeOld(cutoff time.Time) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	cutoffUnix := cutoff.Unix()
	out := sl.List[:0]
	for _, server := range sl.List {
		if cutoffUnix <= server.UpdateTime {
			out = append(out, server)
		}
	}
	if len(out) < len(sl.List) {
		sl.modified = true
	}
	sl.List = out
}

func (sl *ServerList) Load() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	data, err := os.ReadFile(sl.StoragePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var storage storageDocument
	if err := json.Unmarshal(data, &storage); err != nil {
		return err
	}
	sl.List = storage.List
	sl.MaxServers = storage.MaxServers
	sl.MaxClients = storage.MaxClients
	sl.modified = true
	return nil
}

func (sl *ServerList) Save() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if err := sl.saveStorageLocked(); err != nil {
		return err
	}
	if err := sl.savePublicLocked(); err != nil {
		return err
	}
	sl.modified = false
	return nil
}

func (sl *ServerList) saveStorageLocked() error {
	if !sl.modified {
		return nil
	}
	return dumpJSON(sl.StoragePath, storageDocument{
		List:       sl.List,
		MaxServers: sl.MaxServers,
		MaxClients: sl.MaxClients,
	}, sl.debug)
}

func (sl *ServerList) savePublicLocked() error {
	if !sl.modified {
		if _, err := os.Stat(sl.PublicPath); err == nil {
			return nil
		}
	}

	sortedList := append([]*Server(nil), sl.List...)
	sort.Slice(sortedList, func(i, j int) bool {
		return sortedList[i].Score() > sortedList[j].Score()
	})

	outList := make([]map[string]any, 0, len(sortedList))
	clients := 0
	for _, server := range sortedList {
		outList = append(outList, server.ToListJSON())
		clients += int(number(server.Meta["clients"]))
	}
	servers := len(sortedList)
	if servers > sl.MaxServers {
		sl.MaxServers = servers
	}
	if clients > sl.MaxClients {
		sl.MaxClients = clients
	}

	public := map[string]any{
		"total":     map[string]int{"servers": servers, "clients": clients},
		"total_max": map[string]int{"servers": sl.MaxServers, "clients": sl.MaxClients},
		"list":      outList,
	}
	return dumpJSON(sl.PublicPath, public, sl.debug)
}

func (sl *ServerList) Update(server *Server) *Server {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for i, old := range sl.List {
		if old.IP == server.IP && old.Port == server.Port {
			sl.List[i] = server
			sl.modified = true
			return old
		}
	}
	sl.List = append(sl.List, server)
	sl.modified = true
	return nil
}

func dumpJSON(filename string, data any, debug bool) error {
	tmp := filename + "~"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	if debug {
		enc.SetIndent("", "\t")
	}
	if err := enc.Encode(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

type storageDocument struct {
	List       []*Server `json:"list"`
	MaxServers int       `json:"maxServers"`
	MaxClients int       `json:"maxClients"`
}

func (d *storageDocument) UnmarshalJSON(data []byte) error {
	var raw struct {
		List       []json.RawMessage `json:"list"`
		MaxServers int               `json:"maxServers"`
		MaxClients int               `json:"maxClients"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	d.MaxServers = raw.MaxServers
	d.MaxClients = raw.MaxClients
	d.List = make([]*Server, 0, len(raw.List))
	for _, item := range raw.List {
		server, err := unmarshalStoredServer(item)
		if err != nil {
			return err
		}
		d.List = append(d.List, server)
	}
	return nil
}

func (d storageDocument) MarshalJSON() ([]byte, error) {
	type rawDocument struct {
		List       []any `json:"list"`
		MaxServers int   `json:"maxServers"`
		MaxClients int   `json:"maxClients"`
	}
	out := rawDocument{
		List:       make([]any, 0, len(d.List)),
		MaxServers: d.MaxServers,
		MaxClients: d.MaxClients,
	}
	for _, server := range d.List {
		props := map[string]any{
			"startTime":    server.StartTime,
			"updateCount":  server.UpdateCount,
			"updateTime":   server.UpdateTime,
			"totalClients": server.TotalClients,
			"verifyLevel":  server.VerifyLevel,
		}
		out.List = append(out.List, []any{server.IP, server.Port, server.Address, server.Meta, props})
	}
	return json.Marshal(out)
}

func unmarshalStoredServer(data []byte) (*Server, error) {
	var legacy []json.RawMessage
	if err := json.Unmarshal(data, &legacy); err == nil && len(legacy) == 5 {
		var server Server
		if err := json.Unmarshal(legacy[0], &server.IP); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(legacy[1], &server.Port); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(legacy[2], &server.Address); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(legacy[3], &server.Meta); err != nil {
			return nil, err
		}
		var props struct {
			StartTime    int64 `json:"startTime"`
			UpdateCount  int   `json:"updateCount"`
			UpdateTime   int64 `json:"updateTime"`
			TotalClients int   `json:"totalClients"`
			VerifyLevel  int   `json:"verifyLevel"`
		}
		if err := json.Unmarshal(legacy[4], &props); err != nil {
			return nil, err
		}
		server.StartTime = props.StartTime
		server.UpdateCount = props.UpdateCount
		server.UpdateTime = props.UpdateTime
		server.TotalClients = props.TotalClients
		server.VerifyLevel = props.VerifyLevel
		return &server, nil
	}

	var server Server
	if err := json.Unmarshal(data, &server); err != nil {
		return nil, err
	}
	return &server, nil
}
