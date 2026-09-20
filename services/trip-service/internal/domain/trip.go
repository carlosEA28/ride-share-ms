package domain

import (
	"context"
	pb "ride-sharing/shared/proto/trip"
	"ride-sharing/shared/types"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TripModel struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID   string             `json:"userID"`
	Status   string
	RideFare *RideFareModel
	Driver   *pb.TripDriver
}

func (t *TripModel) ToProto() *pb.Trip {
	if t == nil {
		return nil
	}

	trip := &pb.Trip{
		Id:     t.ID.Hex(),
		UserID: t.UserID,
		Status: t.Status,
		Driver: t.Driver,
	}

	if t.RideFare != nil {
		trip.SelectedFare = t.RideFare.ToProto()
		if t.RideFare.Route != nil {
			trip.Route = t.RideFare.Route.ToProto()
		}
	}

	return trip
}

type Trip struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID   string             `json:"userID"`
	Status   string             `json:"status"`
	RideFare *RideFareModel     `json:"rideFare"`
	Driver   *pb.TripDriver     `json:"driver"`
}

func (t *Trip) ToProto() *pb.Trip {
	if t == nil {
		return nil
	}

	trip := &pb.Trip{
		Id:     t.ID.Hex(),
		UserID: t.UserID,
		Status: t.Status,
		Driver: t.Driver,
	}

	if t.RideFare != nil {
		trip.SelectedFare = t.RideFare.ToProto()
		if t.RideFare.Route != nil {
			trip.Route = t.RideFare.Route.ToProto()
		}
	}

	return trip
}

type TripRepository interface {
	CreateTrip(ctx context.Context, fare *TripModel) (*TripModel, error)
	SaveRideFare(ctx context.Context, rideFare *RideFareModel) error
	GetFareByID(ctx context.Context, id string) (*RideFareModel, error)
}

type TripService interface {
	CreateTrip(ctx context.Context, fare *RideFareModel) (*Trip, error)
	GetRoute(ctx context.Context, pickup, destination *types.Coordinate) (*types.OsrmApiResponse, error)
	EstimatePackagesPriceWithRoute(route *types.OsrmApiResponse) []*RideFareModel
	GenerateTripFares(ctx context.Context, fares []*RideFareModel, userID string, Route *types.OsrmApiResponse) ([]*RideFareModel, error)
	GetAndValidateFare(ctx context.Context, fareID, userId string) (*RideFareModel, error)
}
