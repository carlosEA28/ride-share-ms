package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"ride-sharing/services/trip-service/internal/domain"
	tripTypes "ride-sharing/services/trip-service/pkg/types"
	pbd "ride-sharing/shared/proto/driver"
	"ride-sharing/shared/proto/trip"
	"ride-sharing/shared/types"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
		Driver:   &trip.TripDriver{},
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
		Driver:   createdTrip.Driver,
	}, nil
}

func (s *service) GetRoute(ctx context.Context, pickup, destination *types.Coordinate) (*types.OsrmApiResponse, error) {
	url := fmt.Sprintf("http://router.project-osrm.org/route/v1/driving/%f,%f;%f,%f?overview=full&geometries=geojson", pickup.Longitude, pickup.Latitude, destination.Longitude, destination.Latitude)

	log.Printf("Fetching from OSRM API: URL: %s", url)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create OSRM request: %w", err)
	}

	resp, err := otelhttp.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch route from OSRM API: %v", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read the response: %v", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("OSRM API returned status %s: %s", resp.Status, string(body))
	}

	log.Printf("GOT RESPONSE FROM API %s", string(body))

	var routeResponse types.OsrmApiResponse
	if err := json.Unmarshal(body, &routeResponse); err != nil {
		return nil, fmt.Errorf("Failed to parse the response: %v", err)
	}

	if len(routeResponse.Routes) == 0 {
		return nil, fmt.Errorf("OSRM API returned no routes")
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

func (s *service) GenerateTripFares(ctx context.Context, Ridefares []*domain.RideFareModel, userID string, route *types.OsrmApiResponse) ([]*domain.RideFareModel, error) {
	fares := make([]*domain.RideFareModel, 0, len(Ridefares))

	for _, f := range Ridefares {
		id := primitive.NewObjectID()

		fare := &domain.RideFareModel{
			ID:                id,
			UserID:            userID,
			TotalPriceInCents: f.TotalPriceInCents,
			PackageSlug:       f.PackageSlug,
			Route:             (*tripTypes.OsrmApiResponse)(route),
		}

		if err := s.repo.SaveRideFare(ctx, fare); err != nil {
			return nil, fmt.Errorf("Failed to save trip fare: %v", err)
		}

		fares = append(fares, fare)
	}
	return fares, nil
}

func (s *service) GetTripByID(ctx context.Context, id string) (*domain.TripModel, error) {
	return s.repo.GetTripByID(ctx, id)
}

func (s *service) UpdateTrip(ctx context.Context, tripID string, status string, driver *pbd.Driver) error {
	return s.repo.UpdateTrip(ctx, tripID, status, driver)
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

func (s *service) GetAndValidateFare(ctx context.Context, fareID, userID string) (*domain.RideFareModel, error) {
	fare, err := s.repo.GetFareByID(ctx, fareID)
	if err != nil {
		return nil, fmt.Errorf("failed to get trip fare: %w", err)
	}

	if fare == nil {
		return nil, fmt.Errorf("fare does not exist")
	}

	// User fare validation (user is owner of this fare?)
	if userID != fare.UserID {
		return nil, fmt.Errorf("fare does not belong to the user")
	}

	return fare, nil
}
