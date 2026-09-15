package main

import (
	"log"
	"net/http"

	"ride-sharing/shared/env"
)

var (
	httpAddr = env.GetString("HTTP_ADDR", ":8081")
)

func main() {
	log.Println("Starting API Gateway")

	http.HandleFunc("POST /trip/preview", enableCors(handleTripPreview))
	http.HandleFunc("/ws/drivers", handleDriversWebSocket)
	http.HandleFunc("/ws/riders", handleRidersWebSocket)

	http.ListenAndServe(httpAddr, nil)
}
