package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/matthewghuang/ioweyou/internal/api"
	"github.com/matthewghuang/ioweyou/internal/crdt"
	"github.com/matthewghuang/ioweyou/internal/store"
	"github.com/matthewghuang/ioweyou/internal/sync"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "data.db", "SQLite database path")
	staticDir := flag.String("static", "frontend/dist", "frontend static files directory")
	flag.Parse()

	// Initialize store (opens DB, runs schema)
	st, err := store.NewSQLiteStateStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// Initialize HLC
	hlc := crdt.NewHLC()

	// Initialize broadcaster (WebSocket fan-out)
	bcast := sync.NewBroadcaster()

	// Create server dependencies
	srv := &api.Server{
		Store:       st,
		HLC:         hlc,
		AuthDB:      st.DB(),
		Broadcaster: bcast,
	}

	// Create router with frontend
	router := api.NewRouter(srv, *staticDir)

	log.Printf("starting server on %s (db: %s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
