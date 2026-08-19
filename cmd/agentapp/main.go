package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	boardphone "aegis/apps/board/phone"
)

func main() {
	databasePath := os.Getenv("AGENTAPP_DB")
	if databasePath == "" {
		databasePath = "data/agentapp.db"
	}
	if databasePath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
			log.Fatal(err)
		}
	}
	_, _, server, _, err := boardphone.NewPersistentExampleModule(databasePath)
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("AGENTAPP_ADDR")
	if address == "" {
		address = ":8090"
	}
	log.Printf("Agent App module listening on %s (SQLite: %s)", address, databasePath)
	log.Fatal(http.ListenAndServe(address, server))
}
