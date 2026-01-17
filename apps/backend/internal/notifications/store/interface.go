package store

import (
	"context"

	"github.com/kandev/kandev/internal/notifications/models"
)

type Repository interface {
	CreateProvider(ctx context.Context, provider *models.Provider) error
	UpdateProvider(ctx context.Context, provider *models.Provider) error
	GetProvider(ctx context.Context, id string) (*models.Provider, error)
	ListProvidersByUser(ctx context.Context, userID string) ([]*models.Provider, error)
	DeleteProvider(ctx context.Context, id string) error

	ListSubscriptionsByProvider(ctx context.Context, providerID string) ([]*models.Subscription, error)
	ReplaceSubscriptions(ctx context.Context, providerID, userID string, events []string) error

	InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error)
	DeleteDelivery(ctx context.Context, providerID, eventType, taskSessionID string) error

	Close() error
}
