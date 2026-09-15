package main

import (
	"log"
	"net/http"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/util"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleRidersWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
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
	conn, err := upgrader.Upgrade(w, r, nil)
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

	packageSlug := r.URL.Query().Get("packageSlug")
	if packageSlug == "" {
		log.Println("Package Slug is empty")
		return
	}

	type Driver struct {
		Id          string `json:"id"`
		Name        string `json:"name"`
		ProfilePic  string `json:"profilePic"`
		CarPlate    string `json:"carPlate"`
		PackageSlug string `json:"packageSlug"`
	}

	msg := contracts.WSMessage{
		Type: "driver.cmd.register",
		Data: Driver{
			Id:          userID,
			Name:        "Carlos",
			ProfilePic:  util.GetRandomAvatar(1),
			CarPlate:    "ABC1323",
			PackageSlug: packageSlug,
		},
	}

	if err := conn.WriteJSON(msg); err != nil {
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
