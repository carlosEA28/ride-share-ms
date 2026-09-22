package main

import (
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

func handleRidersWebSocket(w http.ResponseWriter, r *http.Request) {
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

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			break
		}

		log.Printf("Received Message: %s", message)
	}

}

func handleDriversWebSocket(w http.ResponseWriter, r *http.Request) {
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

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			break
		}

		log.Printf("Received Message: %s", message)
	}
}
