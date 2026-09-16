package main

import (
	"context" // cria um contexto para controlar o ciclo de vida da aplicação
	"log"     // escreve logs no terminal
	"net"     // abre uma porta de rede
	"os"      // lê sinais do sistema
	"os/signal"
	"ride-sharing/shared/env" // pega variáveis de ambiente com valor padrão
	"syscall"                 // identifica sinais como SIGTERM

	"google.golang.org/grpc" // sobe o servidor gRPC
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

	grpcServer := grpc.NewServer() // cria o servidor gRPC
	// TODO: initialize the grpc handler implementation

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
