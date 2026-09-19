package events

import (
	"context"
	"encoding/json"
	"fmt"
	"ride-sharing/shared/messaging"
)

type TripEventPublisher struct {
	rabbitmq *messaging.RabbitMQ
}

func NewTripEventPublisher(rabbitmq *messaging.RabbitMQ) *TripEventPublisher {
	return &TripEventPublisher{
		rabbitmq: rabbitmq,
	}
}

func (p *TripEventPublisher) PublishTripCreatedEvent(ctx context.Context, tripID, userID, fareID string) error {
	message, err := json.Marshal(struct {
		Type   string `json:"type"`
		TripID string `json:"tripID"`
		UserID string `json:"userID"`
		FareID string `json:"fareID"`
	}{
		Type:   "trip.event.created",
		TripID: tripID,
		UserID: userID,
		FareID: fareID,
	})
	if err != nil {
		return fmt.Errorf("failed to encode trip created event: %w", err)
	}

	return p.rabbitmq.PublishMessage(ctx, "trip.events", string(message))
}
