package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"ride-sharing/shared/env"
)

var (
	httpAddr = env.GetString("HTTP_ADDR", ":8081")
)

func main() {
	log.Println("Starting API Gateway")

	//inicia o tracing
	tracerCfg := tracing.Config{
		ServiceName: "api-gateway",
		Environment: env.GetString("ENVIRONMENT", "development"),
		JaegerURL:   env.GetString("JAEGER_ENDPOINT", env.GetString("JAEGER_URL", "http://jaeger:14268/api/traces")),
	}

	shutdown, err := tracing.InitTracer(tracerCfg)
	if err != nil {
		log.Fatalf("Error initializing tracer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer shutdown(ctx)
	defer cancel()

	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

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

	handler := otelhttp.NewHandler(http.DefaultServeMux, "api-gateway")
	if err := http.ListenAndServe(httpAddr, handler); err != nil {
		log.Fatalf("failed to serve HTTP: %v", err)
	}
}
