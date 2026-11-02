package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/notifications/models"
)

// WebPushRepository persists the VAPID key pair and per-device push
// subscriptions. It is implemented by the same SQLite store as Repository but
// kept separate so notification-provider test doubles need not implement it.
type WebPushRepository interface {
	// LoadOrCreateVAPIDKeys returns the stored key pair, calling generate and
	// persisting its result when none exists. The insert is first-writer-wins,
	// so concurrent callers always observe one pair.
	LoadOrCreateVAPIDKeys(ctx context.Context, generate func() (models.VAPIDKeys, error)) (models.VAPIDKeys, error)
	// UpsertPushSubscription inserts or refreshes a subscription keyed by
	// endpoint. An endpoint already owned by another user is reassigned: a
	// browser installation has one endpoint and signs in as one user at a time.
	UpsertPushSubscription(ctx context.Context, sub *models.PushSubscription) error
	// DeletePushSubscription removes the caller's subscription for an endpoint.
	DeletePushSubscription(ctx context.Context, userID, endpoint string) (bool, error)
	// DeletePushSubscriptionByEndpoint removes an endpoint the push service
	// reported as gone, regardless of owner.
	DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error
	ListPushSubscriptions(ctx context.Context, userID string) ([]*models.PushSubscription, error)
	TouchPushSubscription(ctx context.Context, endpoint string, at time.Time) error
}

var _ WebPushRepository = (*sqliteRepository)(nil)

const webPushSchema = `
	CREATE TABLE IF NOT EXISTS webpush_vapid_keys (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		private_key TEXT NOT NULL,
		public_key TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS webpush_subscriptions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		endpoint TEXT NOT NULL UNIQUE,
		p256dh TEXT NOT NULL,
		auth TEXT NOT NULL,
		user_agent TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_used_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_webpush_subscriptions_user_id ON webpush_subscriptions(user_id);
`

func (r *sqliteRepository) LoadOrCreateVAPIDKeys(ctx context.Context, generate func() (models.VAPIDKeys, error)) (models.VAPIDKeys, error) {
	if keys, ok, err := r.readVAPIDKeys(ctx); err != nil || ok {
		return keys, err
	}
	fresh, err := generate()
	if err != nil {
		return models.VAPIDKeys{}, err
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO webpush_vapid_keys (id, private_key, public_key, created_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`), fresh.PrivateKey, fresh.PublicKey, time.Now().UTC()); err != nil {
		return models.VAPIDKeys{}, err
	}
	keys, ok, err := r.readVAPIDKeys(ctx)
	if err != nil {
		return models.VAPIDKeys{}, err
	}
	if !ok {
		return models.VAPIDKeys{}, errors.New("vapid keys missing after insert")
	}
	return keys, nil
}

// readVAPIDKeys reads from the writer pool so a freshly inserted pair is
// visible without waiting on reader-pool replication.
func (r *sqliteRepository) readVAPIDKeys(ctx context.Context) (models.VAPIDKeys, bool, error) {
	var keys models.VAPIDKeys
	err := r.db.QueryRowContext(ctx,
		`SELECT private_key, public_key FROM webpush_vapid_keys WHERE id = 1`,
	).Scan(&keys.PrivateKey, &keys.PublicKey)
	if errors.Is(err, sql.ErrNoRows) {
		return models.VAPIDKeys{}, false, nil
	}
	if err != nil {
		return models.VAPIDKeys{}, false, err
	}
	return keys, true, nil
}

func (r *sqliteRepository) UpsertPushSubscription(ctx context.Context, sub *models.PushSubscription) error {
	now := time.Now().UTC()
	if sub.ID == "" {
		sub.ID = uuid.New().String()
	}
	sub.CreatedAt = now
	sub.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO webpush_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			user_id = excluded.user_id,
			p256dh = excluded.p256dh,
			auth = excluded.auth,
			user_agent = excluded.user_agent,
			updated_at = excluded.updated_at
	`), sub.ID, sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth, sub.UserAgent, sub.CreatedAt, sub.UpdatedAt)
	return err
}

func (r *sqliteRepository) DeletePushSubscription(ctx context.Context, userID, endpoint string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM webpush_subscriptions WHERE user_id = ? AND endpoint = ?`), userID, endpoint)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (r *sqliteRepository) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM webpush_subscriptions WHERE endpoint = ?`), endpoint)
	return err
}

func (r *sqliteRepository) ListPushSubscriptions(ctx context.Context, userID string) ([]*models.PushSubscription, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`
		SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at, last_used_at
		FROM webpush_subscriptions WHERE user_id = ? ORDER BY created_at ASC
	`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var subs []*models.PushSubscription
	for rows.Next() {
		sub := &models.PushSubscription{}
		var lastUsed sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
			&sub.UserAgent, &sub.CreatedAt, &sub.UpdatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			sub.LastUsedAt = &t
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (r *sqliteRepository) TouchPushSubscription(ctx context.Context, endpoint string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE webpush_subscriptions SET last_used_at = ? WHERE endpoint = ?`), at.UTC(), endpoint)
	return err
}
