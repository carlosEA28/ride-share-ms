package main

import (
	"encoding/json"
	"log"
	"net/http"
	"ride-sharing/services/api-gateway/grpc_clients"
	"ride-sharing/shared/contracts"
)

func handleTripPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqBody previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	// instancia do tripService
	tripService, err := grpc_clients.NewTripServiceClient()
	if err != nil {
		log.Printf("failed to connect to trip service: %v", err)
		http.Error(w, "trip service unavailable", http.StatusServiceUnavailable)
		return
	}

	defer tripService.Close()

	tripPreview, err := tripService.Client.PreviewTrip(r.Context(), reqBody.ToProto())
	if err != nil {
		log.Printf("failed to preview trip: %v", err)
		http.Error(w, "failed to preview trip: "+err.Error(), http.StatusBadGateway)
		return
	}

	response := contracts.APIResponse{Data: tripPreview}

	WriteJson(w, http.StatusCreated, response)
}

func handleTripStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqBody startTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	tripService, err := grpc_clients.NewTripServiceClient()
	if err != nil {
		log.Printf("failed to connect to trip service: %v", err)
		http.Error(w, "trip service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer tripService.Close()

	trip, err := tripService.Client.CreateTrip(r.Context(), reqBody.ToProto())
	if err != nil {
		log.Printf("failed to start trip: %v", err)
		http.Error(w, "failed to start trip", http.StatusBadGateway)
		return
	}

	WriteJson(w, http.StatusCreated, trip)
}
