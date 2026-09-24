package domain

import (
	"context"
	pbd "ride-sharing/shared/proto/driver"
	pb "ride-sharing/shared/proto/trip"
	"ride-sharing/shared/types"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TripModel struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID   string             `bson:"userID" json:"userID"`
	Status   string             `bson:"status"`
	RideFare *RideFareModel     `bson:"rideFare"`
	Driver   *pb.TripDriver     `bson:"driver"`
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
	GetTripByID(ctx context.Context, id string) (*TripModel, error)
	UpdateTrip(ctx context.Context, tripID string, status string, driver *pbd.Driver) error
}

type TripService interface {
	CreateTrip(ctx context.Context, fare *RideFareModel) (*Trip, error)
	GetRoute(ctx context.Context, pickup, destination *types.Coordinate) (*types.OsrmApiResponse, error)
	EstimatePackagesPriceWithRoute(route *types.OsrmApiResponse) []*RideFareModel
	GenerateTripFares(ctx context.Context, fares []*RideFareModel, userID string, Route *types.OsrmApiResponse) ([]*RideFareModel, error)
	GetAndValidateFare(ctx context.Context, fareID, userId string) (*RideFareModel, error)
	GetTripByID(ctx context.Context, tripID string) (*TripModel, error)
	UpdateTrip(ctx context.Context, tripID string, status string, driver *pbd.Driver) error
}
