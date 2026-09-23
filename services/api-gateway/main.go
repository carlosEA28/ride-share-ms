package main

import (
	"log"
	"net/http"
	"os"
	"ride-sharing/shared/messaging"

	"ride-sharing/shared/env"
)

var (
	httpAddr = env.GetString("HTTP_ADDR", ":8081")
)

func main() {
	log.Println("Starting API Gateway")

	//RabbitMQ
	rabbitmq, err := messaging.NewRabbitMQ(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Fatalf("failed to create RabbitMQ instance: %v", err)
	}
	defer rabbitmq.Close()

	http.HandleFunc("/trip/preview", enableCors(handleTripPreview))
	http.HandleFunc("/trip/start", enableCors(handleTripStart))
	http.HandleFunc("/ws/drivers", func(w http.ResponseWriter, r *http.Request) {
		handleDriversWebSocket(w, r, rabbitmq)
	})
	http.HandleFunc("/ws/riders", func(w http.ResponseWriter, r *http.Request) {
		handleRidersWebSocket(w, r, rabbitmq)
	})

	http.ListenAndServe(httpAddr, nil)
}
