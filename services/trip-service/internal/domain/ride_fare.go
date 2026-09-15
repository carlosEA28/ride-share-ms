package domain

import (
	"ride-sharing/services/trip-service/pkg/types"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type RideFareModel struct {
	ID                primitive.ObjectID     `bson:"_id,omitempty"`
	UserID            string                 `bson:"userID"`
	PackageSlug       string                 `bson:"packageSlug"` // ex: van, luxury, sedan
	TotalPriceInCents float64                `bson:"totalPriceInCents"`
	Route             *types.OsrmApiResponse `bson:"route"`
}
