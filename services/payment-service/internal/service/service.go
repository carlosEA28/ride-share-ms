package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ride-sharing/services/payment-service/internal/domain"
	"ride-sharing/services/payment-service/pkg/types"

	"github.com/google/uuid"
)

const (
	// Stripe's maximum amount in minor units (cents)
	maxStripeAmount = 999_999_999
	// Supported currencies (ISO 4217)
	supportedCurrencies = "usd,eur,gbp,brl,cad,aud,mxn"
)

type paymentService struct {
	paymentProcessor domain.PaymentProcessor
}

func NewPaymentService(paymentProcessor domain.PaymentProcessor) domain.Service {
	return &paymentService{
		paymentProcessor: paymentProcessor,
	}
}

func (s *paymentService) CreatePaymentSession(
	ctx context.Context,
	tripID string,
	userID string,
	driverID string,
	amount int64,
	currency string,
) (*types.PaymentIntent, error) {
	// Validate required fields
	if err := s.validateRequiredFields(tripID, userID, driverID); err != nil {
		return nil, err
	}

	// Validate amount
	if err := s.validateAmount(amount); err != nil {
		return nil, err
	}

	// Validate currency
	if err := s.validateCurrency(currency); err != nil {
		return nil, err
	}

	metadata := map[string]string{
		"trip_id":   tripID,
		"user_id":   userID,
		"driver_id": driverID,
	}

	sessionID, err := s.paymentProcessor.CreatePaymentSession(ctx, amount, currency, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment session: %w", err)
	}

	return &types.PaymentIntent{
		ID:              uuid.New().String(),
		TripID:          tripID,
		UserID:          userID,
		DriverID:        driverID,
		Amount:          amount,
		Currency:        currency,
		StripeSessionID: sessionID,
		CreatedAt:       time.Now(),
	}, nil
}

func (s *paymentService) validateRequiredFields(tripID, userID, driverID string) error {
	if strings.TrimSpace(tripID) == "" {
		return fmt.Errorf("trip_id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("user_id is required")
	}
	if strings.TrimSpace(driverID) == "" {
		return fmt.Errorf("driver_id is required")
	}
	return nil
}

func (s *paymentService) validateAmount(amount int64) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if amount > maxStripeAmount {
		return fmt.Errorf("amount exceeds maximum allowed (%d)", maxStripeAmount)
	}
	return nil
}

func (s *paymentService) validateCurrency(currency string) error {
	currency = strings.TrimSpace(strings.ToLower(currency))
	if currency == "" {
		return fmt.Errorf("currency is required")
	}
	// Check if currency is in supported list
	supported := strings.Split(supportedCurrencies, ",")
	for _, c := range supported {
		if c == currency {
			return nil
		}
	}
	return fmt.Errorf("unsupported currency: %s", currency)
}
