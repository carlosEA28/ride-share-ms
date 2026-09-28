package types

import "time"

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSuccess   PaymentStatus = "success"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

type Payment struct {
	ID              string        `json:"id"`
	TripID          string        `json:"trip_id"`
	UserID          string        `json:"user_id"`
	Amount          int64         `json:"amount"`
	Currency        string        `json:"currency"`
	Status          PaymentStatus `json:"status"`
	StripeSessionID string        `json:"stripe_session_id"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type PaymentIntent struct {
	ID              string    `json:"id"`
	TripID          string    `json:"trip_id"`
	UserID          string    `json:"user_id"`
	DriverID        string    `json:"driver_id"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	StripeSessionID string    `json:"stripe_session_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type PaymentConfig struct {
	StripeSecretKey     string `json:"stripeSecretKey"`
	StripeWebhookSecret string `json:"stripeWebhookSecret"`
	Currency            string `json:"currency"`
	SuccessURL          string `json:"successURL"`
	CancelURL           string `json:"cancelURL"`
}
