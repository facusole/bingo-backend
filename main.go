package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/facu/bingo-back/api"
	"github.com/facu/bingo-back/config"
	"github.com/facu/bingo-back/store"
	"github.com/facu/bingo-back/ws"
)

func main() {
	st := store.NewStore()
	hubs := ws.NewHubManager()

	apiHandler := &api.API{Store: st}
	wsHandler := &ws.Handler{Store: st, Hubs: hubs}

	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)
	mux.HandleFunc("/ws", wsHandler.ServeWS)

	// limpieza periódica de salas abandonadas
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			for _, id := range st.CleanupInactiveRooms(30 * time.Minute) {
				hubs.Remove(id)
			}
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("bingo-back listening on %s (CORS origin: %s)", addr, config.AllowedOrigin())
	if err := http.ListenAndServe(addr, api.WithCORS(mux)); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
