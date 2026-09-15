package main

import (
	"log"
	"net/http"

	triphttp "ride-sharing/services/trip-service/internal/infrastructure/http"
	"ride-sharing/services/trip-service/internal/infrastructure/repository"
	"ride-sharing/services/trip-service/internal/service"
	"ride-sharing/shared/env"
)

var (
	httpAddr = env.GetString("HTTP_ADDR", ":8083")
)

func main() {
	log.Println("Starting Trip Service")

	inMemRepo := repository.NewInmemRepository()
	svc := service.NewService(inMemRepo)
	handler := &triphttp.HttpHandler{Service: svc}

	http.HandleFunc("POST /preview", handler.HandleTripPreview)

	err := http.ListenAndServe(httpAddr, nil)
	if err != nil {
		return
	}
}
