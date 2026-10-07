package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ride-sharing/services/payment-service/pkg/types"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockPaymentProcessor is a mock implementation of domain.PaymentProcessor
type MockPaymentProcessor struct {
	mock.Mock
}

func (m *MockPaymentProcessor) CreatePaymentSession(ctx context.Context, amount int64, currency string, metadata map[string]string) (string, error) {
	args := m.Called(ctx, amount, currency, metadata)
	return args.String(0), args.Error(1)
}

// TestMain for goleak detection
func TestMain(m *testing.M) {
	// No goleak needed for these tests as they don't spawn goroutines
	m.Run()
}

func TestCreatePaymentSession(t *testing.T) {
	tests := []struct {
		name           string
		tripID         string
		userID         string
		driverID       string
		amount         int64
		currency       string
		mockSetup      func(*MockPaymentProcessor)
		expectError    bool
		errorContains  string
		validateResult func(*testing.T, *types.PaymentIntent)
	}{
		{
			name:     "successful payment session creation",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500, // 25.00 in cents
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				m.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.MatchedBy(func(metadata map[string]string) bool {
					return metadata["trip_id"] != "" &&
						metadata["user_id"] != "" &&
						metadata["driver_id"] != ""
				})).Return("cs_test_123456", nil).Once()
			},
			expectError: false,
			validateResult: func(t *testing.T, result *types.PaymentIntent) {
				assert.NotEmpty(t, result.ID)
				assert.Equal(t, "cs_test_123456", result.StripeSessionID)
				assert.Equal(t, int64(2500), result.Amount)
				assert.Equal(t, "usd", result.Currency)
				assert.False(t, result.CreatedAt.IsZero())
				assert.WithinDuration(t, time.Now(), result.CreatedAt, time.Second)
			},
		},
		{
			name:     "rejects zero amount",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   0,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "amount must be positive",
		},
		{
			name:     "rejects negative amount",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   -100,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "amount must be positive",
		},
		{
			name:     "rejects excessive amount over Stripe limit",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   1_000_000_000, // Exceeds Stripe's 999,999,999 limit
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "amount exceeds maximum allowed",
		},
		{
			name:     "rejects empty currency",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "currency is required",
		},
		{
			name:     "rejects unsupported currency",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "xyz", // Invalid currency code
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called - validation happens before Stripe call
			},
			expectError:   true,
			errorContains: "unsupported currency",
		},
		{
			name:     "propagates trip_id, user_id, driver_id in metadata to Stripe",
			tripID:   "trip_abc123",
			userID:   "user_xyz789",
			driverID: "driver_def456",
			amount:   5000,
			currency: "eur",
			mockSetup: func(m *MockPaymentProcessor) {
				m.On("CreatePaymentSession", mock.Anything, int64(5000), "eur", mock.MatchedBy(func(metadata map[string]string) bool {
					return metadata["trip_id"] == "trip_abc123" &&
						metadata["user_id"] == "user_xyz789" &&
						metadata["driver_id"] == "driver_def456"
				})).Return("cs_test_metadata", nil).Once()
			},
			expectError: false,
			validateResult: func(t *testing.T, result *types.PaymentIntent) {
				assert.Equal(t, "trip_abc123", result.TripID)
				assert.Equal(t, "user_xyz789", result.UserID)
				assert.Equal(t, "driver_def456", result.DriverID)
			},
		},
		{
			name:     "wraps Stripe error with context",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				m.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.Anything).
					Return("", errors.New("stripe: invalid API key")).Once()
			},
			expectError:   true,
			errorContains: "failed to create payment session",
		},
		{
			name:     "generates valid PaymentIntent ID",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				m.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.Anything).
					Return("cs_test_1", nil).Once()
			},
			expectError: false,
			validateResult: func(t *testing.T, result *types.PaymentIntent) {
				assert.NotEmpty(t, result.ID)
				_, err := uuid.Parse(result.ID)
				assert.NoError(t, err, "ID must be a valid UUID")
			},
		},
		{
			name:     "rejects empty tripID",
			tripID:   "",
			userID:   uuid.New().String(),
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "trip_id is required",
		},
		{
			name:     "rejects empty userID",
			tripID:   uuid.New().String(),
			userID:   "",
			driverID: uuid.New().String(),
			amount:   2500,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "user_id is required",
		},
		{
			name:     "rejects empty driverID",
			tripID:   uuid.New().String(),
			userID:   uuid.New().String(),
			driverID: "",
			amount:   2500,
			currency: "usd",
			mockSetup: func(m *MockPaymentProcessor) {
				// Should not be called
			},
			expectError:   true,
			errorContains: "driver_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockProcessor := new(MockPaymentProcessor)
			tt.mockSetup(mockProcessor)

			svc := NewPaymentService(mockProcessor)

			result, err := svc.CreatePaymentSession(
				context.Background(),
				tt.tripID,
				tt.userID,
				tt.driverID,
				tt.amount,
				tt.currency,
			)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}

			mockProcessor.AssertExpectations(t)
		})
	}
}

func TestCreatePaymentSession_UniqueIDPerCall(t *testing.T) {
	// This test specifically validates that each call generates a unique ID
	mockProcessor := new(MockPaymentProcessor)
	mockProcessor.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.Anything).
		Return("cs_test_1", nil).Once()
	mockProcessor.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.Anything).
		Return("cs_test_2", nil).Once()

	svc := NewPaymentService(mockProcessor)

	tripID := uuid.New().String()
	userID := uuid.New().String()
	driverID := uuid.New().String()

	result1, err := svc.CreatePaymentSession(context.Background(), tripID, userID, driverID, 2500, "usd")
	require.NoError(t, err)
	require.NotNil(t, result1)

	result2, err := svc.CreatePaymentSession(context.Background(), tripID, userID, driverID, 2500, "usd")
	require.NoError(t, err)
	require.NotNil(t, result2)

	// IDs must be different
	assert.NotEqual(t, result1.ID, result2.ID, "each call must generate a unique PaymentIntent ID")

	// Both must be valid UUIDs
	_, err = uuid.Parse(result1.ID)
	assert.NoError(t, err)
	_, err = uuid.Parse(result2.ID)
	assert.NoError(t, err)

	mockProcessor.AssertExpectations(t)
}

func TestCreatePaymentSession_ContextCancellation(t *testing.T) {
	mockProcessor := new(MockPaymentProcessor)
	// Simulate a slow Stripe call that respects context cancellation
	callCount := 0
	mockProcessor.On("CreatePaymentSession", mock.Anything, int64(2500), "usd", mock.Anything).
		Run(func(args mock.Arguments) {
			callCount++
			ctx := args.Get(0).(context.Context)
			// Wait for context cancellation or a short delay
			select {
			case <-ctx.Done():
				// Context cancelled, return error
			case <-time.After(100 * time.Millisecond):
				// Normal completion
			}
		}).
		Return("", context.Canceled).Once()

	svc := NewPaymentService(mockProcessor)

	// Create a context that will be cancelled immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.CreatePaymentSession(ctx, uuid.New().String(), uuid.New().String(), uuid.New().String(), 2500, "usd")

	// Should return the context cancellation error wrapped
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create payment session")

	mockProcessor.AssertExpectations(t)
}