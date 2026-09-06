package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	nhc "nhc-go-client"
)

//go:embed web/*
var webFiles embed.FS

type server struct {
	client      *nhc.Client
	mu          sync.RWMutex
	statusCache statusResponse
	updated     time.Time
	err         error
}

type actionRequest struct {
	ID         int   `json:"id"`
	Brightness *int  `json:"brightness"`
	On         *bool `json:"on"`
}

type statusResponse struct {
	Locations   []nhc.Location   `json:"locations"`
	Actions     []nhc.Action     `json:"actions"`
	Thermostats []nhc.Thermostat `json:"thermostats"`
}

func main() {
	address := flag.String("addr", "127.0.0.1:8765", "GUI listen address")
	flag.Parse()

	config, err := nhc.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	mapper, err := config.Brightness.Mapper()
	if err != nil {
		log.Fatal(err)
	}
	client, err := nhc.NewClient(nhc.Config{
		IP:              config.IP,
		Port:            config.Port,
		Timeout:         config.Timeout,
		BrightnessCurve: mapper,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	app := &server{client: client}
	go app.refreshLoop()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", app.status)
	mux.HandleFunc("/api/actions", app.actions)
	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))

	log.Printf("NHC GUI listening on http://%s", *address)
	log.Fatal(http.ListenAndServe(*address, mux))
}

func (s *server) status(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	status := s.statusCache
	err := s.err
	updated := s.updated
	s.mu.RUnlock()
	if err != nil && updated.IsZero() {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	writer.Header().Set("X-NHC-Status-Age", time.Since(updated).Round(time.Millisecond).String())
	writeJSON(writer, status)
}

func (s *server) refreshLoop() {
	for {
		s.refresh()
		time.Sleep(5 * time.Second)
	}
}

func (s *server) refresh() {
	locations, err := s.client.GetLocations()
	if err != nil {
		s.setError(err)
		return
	}
	actions, err := s.client.GetActions()
	if err != nil {
		s.setError(err)
		return
	}
	thermostats, err := s.client.GetThermostats()
	if err != nil {
		s.setError(err)
		return
	}
	s.mu.Lock()
	s.statusCache = statusResponse{Locations: locations, Actions: actions, Thermostats: thermostats}
	s.updated = time.Now()
	s.err = nil
	s.mu.Unlock()
}

func (s *server) setError(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *server) actions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload actionRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		http.Error(writer, "invalid action request", http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	var action *nhc.Action
	for index := range s.statusCache.Actions {
		if s.statusCache.Actions[index].ID == payload.ID {
			candidate := s.statusCache.Actions[index]
			action = &candidate
			break
		}
	}
	s.mu.RUnlock()
	if action == nil {
		http.Error(writer, "action not found in current snapshot", http.StatusNotFound)
		return
	}
	var err error
	if payload.On != nil && !*payload.On {
		err = action.TurnOff()
	} else if payload.Brightness != nil {
		err = action.TurnOn(*payload.Brightness)
	} else {
		err = action.TurnOn()
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(writer, map[string]string{"status": "ok"})
}

func writeJSON(writer http.ResponseWriter, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
