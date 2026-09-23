package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"ride-sharing/shared/contracts"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	TripExchange = "trip"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	Channel *amqp.Channel
}

type MessageHandler func(context.Context, amqp.Delivery) error

func NewRabbitMQ(uri string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %s", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create channel: %s", err)
	}

	rmq := &RabbitMQ{conn: conn, Channel: channel}

	if err := rmq.setupExchangesAndQueues(); err != nil {
		rmq.Close()

		return nil, fmt.Errorf("failed to setup exchanges and queues: %s", err)
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

	ctx := context.Background()

	go func() {
		for msg := range msgs {
			log.Printf("Received a message: %s", msg.Body)

			if err := handler(ctx, msg); err != nil {
				log.Printf("failed to handle the message: %v", err)
				if nackErr := msg.Nack(false, false); nackErr != nil {
					log.Printf("failed to reject message: %v", nackErr)
				}
				continue
			}

			// Acknowledge the message
			_ = msg.Ack(false)
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

	return r.Channel.PublishWithContext(ctx,
		TripExchange, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			Body:         jsonMsg,
			DeliveryMode: amqp.Persistent,
		})
}

func (r *RabbitMQ) setupExchangesAndQueues() error {

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
	// Declaracao da fila
	q, err := r.Channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
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
