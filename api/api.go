package api

import (
	"encoding/json"
	"net/http"

	"github.com/facu/bingo-back/config"
	"github.com/facu/bingo-back/store"
)

// API groups HTTP handlers that depend on a domain Store.
type API struct {
	Store *store.Store
}

// RegisterRoutes binds all HTTP endpoints onto the given mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("POST /rooms", a.createRoomHandler)
}

// WithCORS sets CORS headers and answers OPTIONS preflight before the mux.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", config.AllowedOrigin())
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func (a *API) createRoomHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		AdminName string `json:"adminName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AdminName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(jsonError{Error: "invalid JSON or missing adminName"})
		return
	}

	room, admin, err := a.Store.CreateRoom(req.AdminName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(jsonError{Error: err.Error()})
		return
	}

	resp := struct {
		ID         string `json:"id"`
		AdminToken string `json:"adminToken"`
	}{
		ID:         string(room.ID),
		AdminToken: string(admin.Token),
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

type jsonError struct {
	Error string `json:"error"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}