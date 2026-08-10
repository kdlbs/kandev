package forgejo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrInvalidWebhookSignature = errors.New("forgejo: invalid webhook signature")

// VerifyWebhookSignature accepts the SHA-256 HMAC representation used by
// Forgejo/Gitea, with or without the conventional sha256= prefix.
func VerifyWebhookSignature(secret string, payload []byte, signature string) error {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) == 0 {
		return ErrInvalidWebhookSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return ErrInvalidWebhookSignature
	}
	return nil
}

// RecordWebhookDelivery provides replay protection. A delivery ID is accepted
// once per workspace; callers should treat false as an idempotent no-op.
func (s *Store) RecordWebhookDelivery(ctx context.Context, workspaceID, deliveryID string, payload []byte) (bool, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(deliveryID) == "" {
		return false, errors.New("forgejo webhook workspace and delivery ID are required")
	}
	sum := sha256.Sum256(payload)
	result, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_webhook_deliveries (workspace_id, delivery_id, payload_hash, created_at) VALUES (?, ?, ?, ?) ON CONFLICT(workspace_id, delivery_id) DO NOTHING`, workspaceID, deliveryID, hex.EncodeToString(sum[:]), time.Now().UTC())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// HandleWebhook verifies and records a delivery, then immediately polls the
// matching issue watches. Polling remains the recovery path for missed hooks.
func (s *Service) HandleWebhook(ctx context.Context, workspaceID, deliveryID, signature string, payload []byte) error {
	if s.secrets == nil {
		return ErrNotConfigured
	}
	secret, err := s.secrets.Reveal(ctx, WebhookSecretKeyForWorkspace(workspaceID))
	if err != nil || strings.TrimSpace(secret) == "" {
		return ErrNotConfigured
	}
	if err := VerifyWebhookSignature(secret, payload, signature); err != nil {
		return err
	}
	fresh, err := s.store.RecordWebhookDelivery(ctx, workspaceID, deliveryID, payload)
	if err != nil || !fresh {
		return err
	}
	watches, err := s.ListIssueWatches(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, watch := range watches {
		if watch.Enabled {
			if _, err := s.PollIssueWatch(ctx, workspaceID, watch.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
