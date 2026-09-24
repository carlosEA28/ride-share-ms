package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"ride-sharing/shared/env"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"
	"syscall"

	grpcserver "google.golang.org/grpc"
)

var GrpcAddr = ":9092"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//inicia o tracing
	tracerCfg := tracing.Config{
		ServiceName: "driver-service",
		Environment: env.GetString("ENVIRONMENT", "development"),
		JaegerURL:   env.GetString("JAEGER_ENDPOINT", env.GetString("JAEGER_URL", "http://jaeger:14268/api/traces")),
	}

	shutdown, err := tracing.InitTracer(tracerCfg)
	if err != nil {
		log.Fatalf("Error initializing tracer: %v", err)
	}

	defer shutdown(ctx)
	defer cancel()

	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	lis, err := net.Listen("tcp", GrpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	svc := NewService()

	//RabbitMQ
	rabbitmq, err := messaging.NewRabbitMQ(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Fatalf("failed to create RabbitMQ instance: %v", err)
	}
	defer rabbitmq.Close()

	//consumer rabbitmq
	consumer := NewTripConsumer(rabbitmq, svc)
	go func() {
		if err := consumer.Listen(); err != nil {
			log.Fatalf("failed to listen for trip messages: %v", err)
		}
	}()

	// Starting the gRPC server
	grpcServer := grpcserver.NewServer(tracing.WithTracingInterceptors()...)
	NewGrpcHandler(grpcServer, svc)

	log.Printf("Starting gRPC server Driver service on port %s", lis.Addr().String())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("failed to serve: %v", err)
			cancel()
		}
	}()

	// wait for the shutdown signal
	<-ctx.Done()
	log.Println("Shutting down the server...")
	grpcServer.GracefulStop()
}
