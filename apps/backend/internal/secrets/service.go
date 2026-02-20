package secrets

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
)

// Service provides business logic and validation for secrets.
type Service struct {
	store  SecretStore
	logger *logger.Logger
}

// NewService creates a new secrets service.
func NewService(store SecretStore, log *logger.Logger) *Service {
	return &Service{
		store:  store,
		logger: log,
	}
}

var envKeyRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func (s *Service) validateCreate(req *CreateSecretRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.EnvKey = strings.TrimSpace(req.EnvKey)

	if req.Name == "" || len(req.Name) > 100 {
		return fmt.Errorf("name must be 1-100 characters")
	}
	if !envKeyRegex.MatchString(req.EnvKey) {
		return fmt.Errorf("env_key must be uppercase letters, digits, and underscores (e.g., MY_API_KEY)")
	}
	if req.Value == "" || len(req.Value) > 10000 {
		return fmt.Errorf("value must be 1-10000 characters")
	}
	if req.Category != "" && !ValidCategories[req.Category] {
		return fmt.Errorf("invalid category: %s", req.Category)
	}
	return nil
}

func (s *Service) validateUpdate(req *UpdateSecretRequest) error {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		req.Name = &name
		if name == "" || len(name) > 100 {
			return fmt.Errorf("name must be 1-100 characters")
		}
	}
	if req.Value != nil && (len(*req.Value) == 0 || len(*req.Value) > 10000) {
		return fmt.Errorf("value must be 1-10000 characters")
	}
	if req.Category != nil && !ValidCategories[*req.Category] {
		return fmt.Errorf("invalid category: %s", *req.Category)
	}
	return nil
}

// Create validates and stores a new secret.
func (s *Service) Create(ctx context.Context, req *CreateSecretRequest) (*SecretListItem, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	category := req.Category
	if category == "" {
		category = CategoryCustom
	}

	secret := &SecretWithValue{
		Secret: Secret{
			Name:     req.Name,
			EnvKey:   req.EnvKey,
			Category: category,
			Metadata: req.Metadata,
		},
		Value: req.Value,
	}

	if err := s.store.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}

	return &SecretListItem{
		ID:        secret.ID,
		Name:      secret.Name,
		EnvKey:    secret.EnvKey,
		Category:  secret.Category,
		Metadata:  secret.Metadata,
		HasValue:  true,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	}, nil
}

// Get retrieves secret metadata.
func (s *Service) Get(ctx context.Context, id string) (*Secret, error) {
	return s.store.Get(ctx, id)
}

// Reveal returns the decrypted secret value.
func (s *Service) Reveal(ctx context.Context, id string) (string, error) {
	return s.store.Reveal(ctx, id)
}

// Update validates and updates a secret.
func (s *Service) Update(ctx context.Context, id string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	if err := s.store.Update(ctx, id, req); err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}

	secret, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &SecretListItem{
		ID:        secret.ID,
		Name:      secret.Name,
		EnvKey:    secret.EnvKey,
		Category:  secret.Category,
		Metadata:  secret.Metadata,
		HasValue:  true,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	}, nil
}

// Delete removes a secret.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// List returns all secrets without values.
func (s *Service) List(ctx context.Context) ([]*SecretListItem, error) {
	return s.store.List(ctx)
}

// ListByCategory returns secrets filtered by category.
func (s *Service) ListByCategory(ctx context.Context, category SecretCategory) ([]*SecretListItem, error) {
	return s.store.ListByCategory(ctx, category)
}
