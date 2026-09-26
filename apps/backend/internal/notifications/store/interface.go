package store

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/notifications/models"
)

// ErrProviderNotFound reports that no provider with the requested ID is
// visible to the requesting user. A provider owned by another user is
// reported the same way as one that does not exist, so an ID probe cannot
// confirm the existence of someone else's provider.
var ErrProviderNotFound = errors.New("notification provider not found")

type Repository interface {
	CreateProvider(ctx context.Context, provider *models.Provider) error
	// UpdateProvider is scoped to provider.UserID.
	UpdateProvider(ctx context.Context, provider *models.Provider) error
	GetProvider(ctx context.Context, userID, id string) (*models.Provider, error)
	ListProvidersByUser(ctx context.Context, userID string) ([]*models.Provider, error)
	ListProviderUserIDs(ctx context.Context) ([]string, error)
	DeleteProvider(ctx context.Context, userID, id string) error

	ListSubscriptionsByProvider(ctx context.Context, providerID string) ([]*models.Subscription, error)
	ReplaceSubscriptions(ctx context.Context, providerID, userID string, events []string) error

	InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error)
	DeleteDelivery(ctx context.Context, providerID, eventType, occurrenceID string) error

	Close() error
}
