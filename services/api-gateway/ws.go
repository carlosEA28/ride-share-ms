package main

import (
	"encoding/json"
	"log"
	"net/http"
	"ride-sharing/services/api-gateway/grpc_clients"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/proto/driver"
)

var (
	connManager = messaging.NewConnectionManager()
)

func handleRidersWebSocket(w http.ResponseWriter, r *http.Request, rb *messaging.RabbitMQ) {
	conn, err := connManager.Upgrader(w, r)
	if err != nil {
		log.Println(err)
		return
	}

	defer conn.Close()

	userID := r.URL.Query().Get("userID")
	if userID == "" {
		log.Println("userID is empty")
		return
	}

	//inicia consumer das filas
	queues := []string{
		messaging.NotifyDriverNoDriversFoundQueue,
		messaging.NotifyDriverAssignQueue,
	}

	//consome elas
	for _, queue := range queues {
		consumer := messaging.NewQueueConsumer(rb, connManager, queue)
		if err := consumer.Start(); err != nil {
			log.Println(err)
		}
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			break
		}

		log.Printf("Received Message: %s", message)
	}

}

func handleDriversWebSocket(w http.ResponseWriter, r *http.Request, rb *messaging.RabbitMQ) {
	conn, err := connManager.Upgrader(w, r)
	if err != nil {
		log.Println(err)
		return
	}

	defer conn.Close()

	userID := r.URL.Query().Get("userID")
	if userID == "" {
		log.Println("userID is empty")
		return
	}

	//adiciona conection no manager
	connManager.Add(userID, conn)

	packageSlug := r.URL.Query().Get("packageSlug")
	if packageSlug == "" {
		log.Println("Package Slug is empty")
		return
	}

	//adiciona conection no manager
	connManager.Add(userID, conn)

	ctx := r.Context()

	driverService, err := grpc_clients.NewDriverServiceClient()
	if err != nil {
		log.Println(err)
	}

	defer func() {
		defer connManager.Remove(userID)
		driverService.Client.UnregisterDriver(ctx, &driver.RegisterDriverRequest{DriverID: userID, PackageSlug: packageSlug})
		driverService.Close()

		log.Println("Unregistered Driver: ", userID)
	}()

	driverData, err := driverService.Client.RegisterDriver(ctx, &driver.RegisterDriverRequest{DriverID: userID, PackageSlug: packageSlug})
	if err != nil {
		log.Println(err)
		return
	}

	if err := connManager.SendMessage(userID, contracts.WSMessage{
		Type: contracts.DriverCmdRegister,
		Data: driverData.Driver,
	}); err != nil {
		log.Println(err)
		return
	}

	//inicia consumer das filas
	queues := []string{
		messaging.DriverCmdTripRequestQueue,
	}

	//consome elas
	for _, queue := range queues {
		consumer := messaging.NewQueueConsumer(rb, connManager, queue)
		if err := consumer.Start(); err != nil {
			log.Println(err)
		}
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			break
		}

		type DriverMessage struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}

		var driverMsg DriverMessage
		if err := json.Unmarshal(message, &driverMsg); err != nil {
			log.Println(err)
			continue
		}

		switch driverMsg.Type {
		case contracts.DriverCmdLocation:
			//Handle driver location in the future
			continue
		case contracts.DriverCmdTripAccept, contracts.DriverCmdTripDecline:
			if err := rb.PublishMessage(ctx, driverMsg.Type, contracts.AmqpMessage{
				OwnerID: userID,
				Data:    driverMsg.Data,
			}); err != nil {
				log.Printf("Error publishing message: %v", err)
			}
		default:
			log.Printf("Unknown message type: %s", driverMsg.Type)

		}
	}
}
