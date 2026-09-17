package grpc

import (
	"context"
	"ride-sharing/services/trip-service/internal/domain"
	pb "ride-sharing/shared/proto/trip"
	"ride-sharing/shared/types"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GrpcHandler struct {
	pb.UnimplementedTripServiceServer
	service domain.TripService
}

func NewGrpcHandler(server *grpc.Server, service domain.TripService) *GrpcHandler {
	handler := &GrpcHandler{
		service: service,
	}

	pb.RegisterTripServiceServer(server, handler)
	return handler
}

func (h *GrpcHandler) PreviewTrip(ctx context.Context, request *pb.PreviewTripRequest) (*pb.PreviewTripResponse, error) {

	pickup := request.GetStartLocation()
	destination := request.GetEndLocation()

	pickupCords := &types.Coordinate{
		Latitude:  pickup.Latitude,
		Longitude: pickup.Longitude,
	}

	destinationCords := &types.Coordinate{
		Latitude:  destination.Latitude,
		Longitude: destination.Longitude,
	}

	userID := request.GetUserID()

	route, err := h.service.GetRoute(ctx, pickupCords, destinationCords)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch route: %w", err)
	}

	//faz a estimativa do preco da corrida baseado na rota( ex: distancia)
	estimatedFares := h.service.EstimatePackagesPriceWithRoute(route)

	//salva o preco da corrida criada
	fares, err := h.service.GenerateTripFares(ctx, estimatedFares, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate trip fares: %w", err)
	}

	return &pb.PreviewTripResponse{
		Route:     route.ToProto(),
		RideFares: domain.ToRideFaresProto(fares),
	}, nil
}
