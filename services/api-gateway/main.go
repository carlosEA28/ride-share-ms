package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"

	"ride-sharing/shared/env"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

	http.Handle("/trip/preview", tracing.WrapperHandlerFunc(enableCors(handleTripPreview), "/trip/preview"))
	http.Handle("/trip/start", tracing.WrapperHandlerFunc(enableCors(handleTripStart), "/trip/start"))
	http.Handle("/ws/drivers", tracing.WrapperHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleDriversWebSocket(w, r, rabbitmq)
	}, "/ws/drivers"))
	http.Handle("/ws/riders", tracing.WrapperHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRidersWebSocket(w, r, rabbitmq)
	}, "/ws/riders"))

	handler := otelhttp.NewHandler(http.DefaultServeMux, "api-gateway")
	if err := http.ListenAndServe(httpAddr, handler); err != nil {
		log.Fatalf("failed to serve HTTP: %v", err)
	}
}
