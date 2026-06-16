package main

import (
	"net/http"
	"time"

	"github.com/facu/bingo-back/api"
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

	http.ListenAndServe(":8080", api.WithCORS(mux))
}