package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"ride-sharing/services/trip-service/internal/infrastructure/events"
	"ride-sharing/services/trip-service/internal/infrastructure/grpc"
	"ride-sharing/services/trip-service/internal/infrastructure/repository"
	"ride-sharing/services/trip-service/internal/service"
	"ride-sharing/shared/env"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"
	"syscall"

	grpc_server "google.golang.org/grpc"
)

var (
	GrpcAddr = env.GetString("GRPC_ADDR", ":9093") // endereço gRPC configurado por variável de ambiente
)

func main() {
	log.Println("Starting Trip Service")

	ctx, cancel := context.WithCancel(context.Background()) // cria contexto cancelável
	defer cancel()                                          // garante liberação do cancelamento no fim

	go func() {
		sigChan := make(chan os.Signal, 1)                    // cria canal para receber sinais
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM) // escuta interrupção e término
		<-sigChan                                             // espera um sinal chegar
		cancel()                                              // cancela o contexto quando receber sinal
	}()

	//inicia o tracing
	tracerCfg := tracing.Config{
		ServiceName: "trip-service",
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

	repo := repository.NewInmemRepository()
	svc := service.NewService(repo)

	listener, err := net.Listen("tcp", GrpcAddr) // abre a porta do servidor gRPC
	if err != nil {
		log.Fatalf("failed to listen: %v", err) // encerra se não conseguir abrir a porta
	}

	//RabbitMQ
	rabbitmq, err := messaging.NewRabbitMQ(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Fatalf("failed to create RabbitMQ instance: %v", err)
	}
	defer rabbitmq.Close()
	publisher := events.NewTripEventPublisher(rabbitmq)

	// inicia o consumer do driver
	driverConsumer := events.NewDriverConsumer(rabbitmq, svc)
	go driverConsumer.Listen()

	grpcServer := grpc_server.NewServer(tracing.WithTracingInterceptors()...) // cria o servidor gRPC

	grpc.NewGrpcHandler(grpcServer, svc, publisher)

	log.Println("Starting gRPC Server [Trip Service] on port 9093") // registra a porta usada

	go func() {
		if err := grpcServer.Serve(listener); err != nil { // começa a atender requisições gRPC
			log.Fatalf("failed to serve: %v", err) // encerra se o servidor falhar
			cancel()                               // cancela o contexto após erro
		}
	}()

	// espera o sinal de desligamento
	<-ctx.Done()                              // bloqueia até o contexto ser cancelado
	log.Println("Shutting down Trip Service") // avisa que vai encerrar
	grpcServer.GracefulStop()                 // finaliza o servidor de forma segura
}
