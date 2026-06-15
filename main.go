package main

import (
	"net/http"

	"github.com/facu/bingo-back/api"
	"github.com/facu/bingo-back/store"
)

func main() {
	apiInstance := &api.API{Store: store.NewStore()}
	mux := http.NewServeMux()
	apiInstance.RegisterRoutes(mux)
	http.ListenAndServe(":8080", api.WithCORS(mux))
}