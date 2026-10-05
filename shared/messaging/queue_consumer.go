package messaging

import (
	"context"
	"encoding/json"
	"log"

	"ride-sharing/shared/contracts"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type QueueConsumer struct {
	rb        *RabbitMQ
	connMgr   *ConnectionManager
	queueName string
}

func NewQueueConsumer(rb *RabbitMQ, connMgr *ConnectionManager, queueName string) *QueueConsumer {
	return &QueueConsumer{
		rb:        rb,
		connMgr:   connMgr,
		queueName: queueName,
	}
}

func (qc *QueueConsumer) Start() error {
	msgs, err := qc.rb.Channel.Consume(
		qc.queueName,
		"",
		true, // auto-ack
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	tracer := otel.GetTracerProvider().Tracer("messaging")

	go func() {
		for msg := range msgs {
			// Extract trace context from message headers
			headers := make(map[string]interface{})
			for k, v := range msg.Headers {
				headers[k] = v
			}
			ctx := ExtractHeaders(context.Background(), headers)

			// Start consumer span for forwarding
			ctx, span := tracer.Start(ctx, "forward",
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "rabbitmq"),
					attribute.String("messaging.destination", qc.queueName),
					attribute.String("messaging.destination_kind", "queue"),
					attribute.String("messaging.operation", "forward"),
					attribute.String("messaging.routing_key", msg.RoutingKey),
				),
			)

			var msgBody contracts.AmqpMessage
			if err := json.Unmarshal(msg.Body, &msgBody); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Println("Failed to unmarshal message:", err)
				span.End()
				continue
			}

			userID := msgBody.OwnerID

			var payload any
			if msgBody.Data != nil {
				if err := json.Unmarshal(msgBody.Data, &payload); err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					log.Println("Failed to unmarshal payload:", err)
					span.End()
					continue
				}
			}

			clientMsg := contracts.WSMessage{
				Type: msg.RoutingKey,
				Data: payload,
			}

			if err := qc.connMgr.SendMessage(userID, clientMsg); err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Printf("Failed to send message to user %s: %v", userID, err)
			}

			span.End()
		}
	}()

	return nil
}
