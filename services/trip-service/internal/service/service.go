package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ride-sharing/services/trip-service/internal/domain"
	tripTypes "ride-sharing/services/trip-service/pkg/types"
	"ride-sharing/shared/types"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type service struct {
	repo domain.TripRepository
}

func NewService(repo domain.TripRepository) *service {
	return &service{
		repo: repo,
	}
}

func (s *service) CreateTrip(ctx context.Context, fare *domain.RideFareModel) (*domain.Trip, error) {
	t := &domain.TripModel{
		ID:       primitive.NewObjectID(),
		UserID:   fare.UserID,
		Status:   "pending",
		RideFare: fare,
	}

	createdTrip, err := s.repo.CreateTrip(ctx, t)
	if err != nil {
		return nil, err
	}

	return &domain.Trip{
		ID:       createdTrip.ID,
		UserID:   createdTrip.UserID,
		Status:   createdTrip.Status,
		RideFare: createdTrip.RideFare,
	}, nil
}

func (s *service) GetRoute(ctx context.Context, pickup, destination *types.Coordinate) (*types.OsrmApiResponse, error) {
	url := fmt.Sprintf("http://router.project-osrm.org/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson", pickup.Longitude, pickup.Latitude, destination.Longitude, destination.Latitude)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch route from OSRM API: %v", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read the response: %v", err)
	}

	var routeResponse types.OsrmApiResponse
	if err := json.Unmarshal(body, &routeResponse); err != nil {
		return nil, fmt.Errorf("Failed to parse the response: %v", err)
	}

	return &routeResponse, nil
}

func (s *service) EstimatePackagesPriceWithRoute(route *types.OsrmApiResponse) []*domain.RideFareModel {
	// Implementation for estimating package prices based on the route
	baseFares := getBasesFare()
	estimatedFares := make([]*domain.RideFareModel, len(baseFares))

	for i, f := range baseFares {
		estimatedFares[i] = estimateFareRoute(f, route)
	}

	return estimatedFares
}

func (s *service) GenerateTripFares(ctx context.Context, Ridefares []*domain.RideFareModel, userID string) ([]*domain.RideFareModel, error) {
	fares := make([]*domain.RideFareModel, len(Ridefares))

	for i, f := range fares {
		id := primitive.NewObjectID()

		fare := &domain.RideFareModel{
			ID:                id,
			UserID:            userID,
			TotalPriceInCents: f.TotalPriceInCents,
			PackageSlug:       f.PackageSlug,
		}

		if err := s.repo.SaveRideFare(ctx, fare); err != nil {
			return nil, fmt.Errorf("Failed to save trip fare: %v", err)
		}

		fares[i] = fare
	}
	return fares, nil
}

// utils functions
func estimateFareRoute(fare *domain.RideFareModel, route *types.OsrmApiResponse) *domain.RideFareModel {
	pricingCfg := tripTypes.DefaultPricingConfig()
	carPackagePrice := fare.TotalPriceInCents
	distanceInKm := route.Routes[0].Distance
	durationInMin := route.Routes[0].Duration

	distanceFare := distanceInKm * pricingCfg.PricePerUnitOfDistance
	timeFare := durationInMin * pricingCfg.PricingPerMinute

	totalPrice := carPackagePrice + distanceFare + timeFare

	return &domain.RideFareModel{
		TotalPriceInCents: totalPrice,
		PackageSlug:       fare.PackageSlug,
	}
}

func getBasesFare() []*domain.RideFareModel {
	return []*domain.RideFareModel{
		{
			PackageSlug:       "suv",
			TotalPriceInCents: 200,
		},
		{
			PackageSlug:       "sedan",
			TotalPriceInCents: 350,
		},
		{
			PackageSlug:       "van",
			TotalPriceInCents: 400,
		},
		{
			PackageSlug:       "luxury",
			TotalPriceInCents: 1000,
		},
	}
}
