package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"ride-sharing/services/trip-service/internal/infrastructure/grpc"
	"ride-sharing/shared/env"
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

	listener, err := net.Listen("tcp", GrpcAddr) // abre a porta do servidor gRPC
	if err != nil {
		log.Fatalf("failed to listen: %v", err) // encerra se não conseguir abrir a porta
	}

	grpcServer := grpc_server.NewServer() // cria o servidor gRPC
	// TODO: passar o service real
	grpc.NewGrpcHandler(grpcServer, nil)

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
