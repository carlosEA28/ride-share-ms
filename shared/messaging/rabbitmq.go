package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/retry"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	TripExchange       = "trip"
	DeadLetterExchange = "dlx"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	Channel *amqp.Channel
}

type MessageHandler func(context.Context, amqp.Delivery) error

func NewRabbitMQ(uri string) (*RabbitMQ, error) {
	var rmq *RabbitMQ

	cfg := retry.DefaultConfig()
	err := retry.WithBackoff(context.Background(), cfg, func() error {
		conn, err := amqp.Dial(uri)
		if err != nil {
			return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
		}

		channel, err := conn.Channel()
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to create channel: %w", err)
		}

		rmq = &RabbitMQ{conn: conn, Channel: channel}

		if err := rmq.setupExchangesAndQueues(); err != nil {
			rmq.Close()
			return fmt.Errorf("failed to setup exchanges and queues: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return rmq, nil
}

func (r *RabbitMQ) ConsumeMessages(queueName string, handler MessageHandler) error {
	msgs, err := r.Channel.Consume(
		queueName, // queue
		"",        // consumer
		false,     // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		return err
	}

	cfg := retry.DefaultConfig()
	tracer := otel.GetTracerProvider().Tracer("messaging")

	go func() {
		for msg := range msgs {
			log.Printf("Received a message: %s", msg.Body)

			// Extract trace context from message headers
			headers := make(map[string]interface{})
			for k, v := range msg.Headers {
				headers[k] = v
			}
			ctx := ExtractHeaders(context.Background(), headers)

			// Start consumer span
			ctx, span := tracer.Start(ctx, "consume",
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "rabbitmq"),
					attribute.String("messaging.destination", queueName),
					attribute.String("messaging.destination_kind", "queue"),
					attribute.String("messaging.operation", "consume"),
				),
			)

			err := retry.WithBackoff(ctx, cfg, func() error {
				return handler(ctx, msg)
			})
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				log.Printf("failed to handle the message: %v", err)
				if nackErr := msg.Nack(false, false); nackErr != nil {
					log.Printf("failed to reject message: %v", nackErr)
				}
				span.End()
				continue
			}

			// Acknowledge the message
			_ = msg.Ack(false)
			span.End()
		}
	}()

	return nil
}

func (r *RabbitMQ) PublishMessage(ctx context.Context, routingKey string, message contracts.AmqpMessage) error {
	log.Printf("Publishing message with routing key: %s", routingKey)

	jsonMsg, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	// Start producer span
	tracer := otel.GetTracerProvider().Tracer("messaging")
	ctx, span := tracer.Start(ctx, "publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", TripExchange),
			attribute.String("messaging.destination_kind", "topic"),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.routing_key", routingKey),
		),
	)
	defer span.End()

	// Inject trace context into headers
	headers := make(map[string]interface{})
	InjectHeaders(ctx, headers)

	return r.Channel.PublishWithContext(ctx,
		TripExchange, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			Body:         jsonMsg,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
		})
}

func (r *RabbitMQ) setupExchangesAndQueues() error {

	// First setup the DLQ exchange and queue
	if err := r.setupDeadLetterExchange(); err != nil {
		return err
	}

	// Declaracao da Exchange
	err := r.Channel.ExchangeDeclare(
		TripExchange, // name
		"topic",      // type
		true,         // durable
		false,        // auto-delete
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to setup exchange: %s", err)
	}

	//BINDS
	if err = r.declareAndBindQueue(FindAvailableDriversQueue, []string{contracts.TripEventCreated, contracts.TripEventDriverNotInterested}, TripExchange); err != nil {
		return fmt.Errorf("failed to setup queue: %s", err)
	}

	if err = r.declareAndBindQueue(DriverCmdTripRequestQueue, []string{contracts.DriverCmdTripRequest}, TripExchange); err != nil {
		return fmt.Errorf("failed to setup queue: %s", err)
	}

	if err = r.declareAndBindQueue(DriverTripResponseQueue, []string{contracts.DriverCmdTripAccept, contracts.DriverCmdTripDecline}, TripExchange); err != nil {
		return fmt.Errorf("failed to setup queue: %s", err)
	}

	if err = r.declareAndBindQueue(NotifyDriverNoDriversFoundQueue, []string{contracts.TripEventNoDriversFound}, TripExchange); err != nil {
		return fmt.Errorf("failed to setup queue: %s", err)
	}

	if err := r.declareAndBindQueue(
		NotifyDriverAssignQueue,
		[]string{contracts.TripEventDriverAssigned},
		TripExchange,
	); err != nil {
		return err
	}

	if err := r.declareAndBindQueue(
		PaymentTripResponseQueue,
		[]string{contracts.PaymentCmdCreateSession},
		TripExchange,
	); err != nil {
		return err
	}

	if err := r.declareAndBindQueue(
		NotifyPaymentSessionCreatedQueue,
		[]string{contracts.PaymentEventSessionCreated},
		TripExchange,
	); err != nil {
		return err
	}

	if err := r.declareAndBindQueue(
		NotifyPaymentSuccessQueue,
		[]string{contracts.PaymentEventSuccess},
		TripExchange,
	); err != nil {
		return err
	}

	return nil
}

func (r *RabbitMQ) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
	if r.Channel != nil {
		r.Channel.Close()
	}
}

// Helper Functions
func (r *RabbitMQ) declareAndBindQueue(queueName string, messageTypes []string, exchange string) error {

	// Add dead letter configuration
	args := amqp.Table{
		"x-dead-letter-exchange": DeadLetterExchange,
	}

	// Declaracao da fila
	q, err := r.Channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		args,      // arguments
	)
	if err != nil {
		log.Fatalf("failed to setup queue: %s", err)
	}

	// Bind da fila com a exchange
	// O loop permite associar a mesma fila a vários tipos de mensagem (routing keys).
	for _, messageType := range messageTypes {
		err = r.Channel.QueueBind(
			q.Name,      // queue
			messageType, // routing key
			exchange,    // exchange
			false,       // no-wait
			nil,         // arguments
		)
		if err != nil {
			log.Fatalf("failed to bind queue: %s", err)
		}
	}

	return nil
}

func (r *RabbitMQ) setupDeadLetterExchange() error {
	// Declare the dead letter exchange
	err := r.Channel.ExchangeDeclare(
		DeadLetterExchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare dead letter exchange: %v", err)
	}

	// Declare the dead letter queue
	q, err := r.Channel.QueueDeclare(
		DeadLetterQueue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare dead letter queue: %v", err)
	}

	// Bind the queue to the exchange with a wildcard routing key
	err = r.Channel.QueueBind(
		q.Name,
		"#", // wildcard routing key to catch all messages
		DeadLetterExchange,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind dead letter queue: %v", err)
	}

	return nil
}
