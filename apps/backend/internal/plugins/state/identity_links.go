package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrIdentityLinkNotFound      = errors.New("plugin identity link was not found")
	ErrDelegatedResponseNotFound = errors.New("plugin delegated human response was not found")
)

// IdentityLink binds one external identity, as seen by one plugin
// installation, to one Kandev user. A subject links to at most one user and a
// user to at most one subject per installation and provider.
type IdentityLink struct {
	ID             string `db:"link_id"`
	InstallationID string `db:"installation_id"`
	Provider       string `db:"provider"`
	Subject        string `db:"subject"`
	DisplayName    string `db:"display_name"`
	UserID         string `db:"user_id"`
	CreatedAt      string `db:"created_at"`
}

// DelegatedResponse is the durable audit record of one human-response receipt
// minted for a linked Kandev user through a plugin installation.
type DelegatedResponse struct {
	ReceiptID       string `db:"receipt_id"`
	InstallationID  string `db:"installation_id"`
	WorkspaceID     string `db:"workspace_id"`
	LinkID          string `db:"link_id"`
	UserID          string `db:"user_id"`
	UserDisplayName string `db:"user_display_name"`
	Provider        string `db:"provider"`
	Subject         string `db:"subject"`
	Kind            string `db:"kind"`
	TargetID        string `db:"target_id"`
	TargetVersion   string `db:"target_version"`
	PayloadDigest   string `db:"payload_digest"`
	IssuedAt        string `db:"issued_at"`
	ExpiresAt       string `db:"expires_at"`
	ConsumedAt      string `db:"consumed_at"`
	// ClaimedAt is written before the response is delivered, so a receipt whose
	// consumed marker was lost after delivery is still identifiable.
	ClaimedAt string `db:"claimed_at"`
}

const identityLinkColumns = `link_id, installation_id, provider, subject, display_name, user_id, created_at`

const delegatedResponseColumns = `receipt_id, installation_id, workspace_id, link_id, user_id, user_display_name,
	provider, subject, kind, target_id, target_version, payload_digest, issued_at, expires_at, consumed_at, claimed_at`

func (s *CommandStore) initIdentityLinkSchema() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_identity_links (
			link_id TEXT PRIMARY KEY,
			installation_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			subject TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE (installation_id, provider, subject),
			UNIQUE (installation_id, provider, user_id)
		);
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_delegated_human_responses (
			receipt_id TEXT PRIMARY KEY,
			installation_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			link_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			user_display_name TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL,
			subject TEXT NOT NULL,
			kind TEXT NOT NULL,
			target_id TEXT NOT NULL,
			target_version TEXT NOT NULL,
			payload_digest TEXT NOT NULL,
			issued_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			consumed_at TEXT NOT NULL DEFAULT '',
			claimed_at TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_plugin_delegated_human_responses_target
			ON plugin_delegated_human_responses(target_id, consumed_at);
	`)
	return err
}

// LinkIdentity stores a link and replaces any link of the same subject or the
// same user for that installation and provider, in one transaction.
func (s *CommandStore) LinkIdentity(ctx context.Context, link IdentityLink) (IdentityLink, error) {
	if link.InstallationID == "" || link.Provider == "" || link.Subject == "" || link.UserID == "" {
		return IdentityLink{}, errors.New("plugin identity link: required fields are missing")
	}
	link.ID = uuid.NewString()
	link.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return IdentityLink{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM plugin_identity_links
		WHERE installation_id = ? AND provider = ? AND (subject = ? OR user_id = ?)
	`), link.InstallationID, link.Provider, link.Subject, link.UserID); err != nil {
		return IdentityLink{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO plugin_identity_links (`+identityLinkColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), link.ID, link.InstallationID, link.Provider, link.Subject, link.DisplayName, link.UserID, link.CreatedAt); err != nil {
		return IdentityLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return IdentityLink{}, err
	}
	return link, nil
}

// FindIdentityLink returns the active link of one external subject.
func (s *CommandStore) FindIdentityLink(ctx context.Context, installationID, provider, subject string) (IdentityLink, error) {
	var link IdentityLink
	err := s.ro.GetContext(ctx, &link, s.ro.Rebind(`SELECT `+identityLinkColumns+`
		FROM plugin_identity_links WHERE installation_id = ? AND provider = ? AND subject = ?`),
		installationID, provider, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityLink{}, ErrIdentityLinkNotFound
	}
	return link, err
}

// GetIdentityLink returns one link of an installation by ID.
func (s *CommandStore) GetIdentityLink(ctx context.Context, installationID, linkID string) (IdentityLink, error) {
	var link IdentityLink
	err := s.ro.GetContext(ctx, &link, s.ro.Rebind(`SELECT `+identityLinkColumns+`
		FROM plugin_identity_links WHERE installation_id = ? AND link_id = ?`), installationID, linkID)
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityLink{}, ErrIdentityLinkNotFound
	}
	return link, err
}

// ListIdentityLinks returns an installation's links in creation order. A
// non-empty userID narrows the result to that user's links.
func (s *CommandStore) ListIdentityLinks(ctx context.Context, installationID, userID string) ([]IdentityLink, error) {
	query := `SELECT ` + identityLinkColumns + ` FROM plugin_identity_links WHERE installation_id = ?`
	args := []any{installationID}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at, link_id`
	links := []IdentityLink{}
	if err := s.ro.SelectContext(ctx, &links, s.ro.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list plugin identity links: %w", err)
	}
	return links, nil
}

// DeleteIdentityLink removes one link of an installation.
func (s *CommandStore) DeleteIdentityLink(ctx context.Context, installationID, linkID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM plugin_identity_links WHERE installation_id = ? AND link_id = ?
	`), installationID, linkID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrIdentityLinkNotFound
	}
	return nil
}

// DeleteInstallationIdentityLinks removes every link of an installation. The
// delegated response audit stays.
func (s *CommandStore) DeleteInstallationIdentityLinks(ctx context.Context, installationID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM plugin_identity_links WHERE installation_id = ?
	`), installationID)
	return err
}

// RecordDelegatedResponse stores the audit record of a newly minted receipt.
func (s *CommandStore) RecordDelegatedResponse(ctx context.Context, record DelegatedResponse) error {
	if record.ReceiptID == "" || record.InstallationID == "" || record.WorkspaceID == "" || record.UserID == "" ||
		record.Kind == "" || record.TargetID == "" || record.PayloadDigest == "" {
		return errors.New("plugin delegated human response: required fields are missing")
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO plugin_delegated_human_responses (`+delegatedResponseColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '')
	`), record.ReceiptID, record.InstallationID, record.WorkspaceID, record.LinkID, record.UserID, record.UserDisplayName,
		record.Provider, record.Subject, record.Kind, record.TargetID, record.TargetVersion, record.PayloadDigest,
		record.IssuedAt, record.ExpiresAt)
	return err
}

// ClaimDelegatedResponse durably records that a receipt is about to be
// delivered. It runs before the native response so the receipt stays
// identifiable if the consumed marker cannot be written afterwards.
func (s *CommandStore) ClaimDelegatedResponse(ctx context.Context, receiptID string) error {
	return s.updateDelegatedResponse(ctx, receiptID,
		`UPDATE plugin_delegated_human_responses SET claimed_at = ? WHERE receipt_id = ? AND claimed_at = '' AND consumed_at = ''`,
		time.Now().UTC().Format(time.RFC3339Nano))
}

// ReleaseDelegatedResponseClaim withdraws a claim whose delivery failed.
func (s *CommandStore) ReleaseDelegatedResponseClaim(ctx context.Context, receiptID string) error {
	return s.updateDelegatedResponse(ctx, receiptID,
		`UPDATE plugin_delegated_human_responses SET claimed_at = ? WHERE receipt_id = ? AND consumed_at = '' AND claimed_at <> ''`, "")
}

func (s *CommandStore) updateDelegatedResponse(ctx context.Context, receiptID, query string, value string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), value, receiptID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDelegatedResponseNotFound
	}
	return nil
}

// ClaimedDelegatedResponse returns the newest receipt claimed for a target
// whose consumed marker is missing, if any.
func (s *CommandStore) ClaimedDelegatedResponse(ctx context.Context, targetID string) (DelegatedResponse, error) {
	var record DelegatedResponse
	err := s.ro.GetContext(ctx, &record, s.ro.Rebind(`SELECT `+delegatedResponseColumns+`
		FROM plugin_delegated_human_responses WHERE target_id = ? AND consumed_at = '' AND claimed_at <> ''
		ORDER BY claimed_at DESC LIMIT 1`), targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return DelegatedResponse{}, ErrDelegatedResponseNotFound
	}
	return record, err
}

// MarkDelegatedResponseConsumed records when a receipt resolved its target.
func (s *CommandStore) MarkDelegatedResponseConsumed(ctx context.Context, receiptID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_delegated_human_responses SET consumed_at = ? WHERE receipt_id = ? AND consumed_at = ''
	`), time.Now().UTC().Format(time.RFC3339Nano), receiptID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDelegatedResponseNotFound
	}
	return nil
}

// ConsumedDelegatedResponse returns the delegated response that resolved a
// target, if one did.
func (s *CommandStore) ConsumedDelegatedResponse(ctx context.Context, targetID string) (DelegatedResponse, error) {
	var record DelegatedResponse
	err := s.ro.GetContext(ctx, &record, s.ro.Rebind(`SELECT `+delegatedResponseColumns+`
		FROM plugin_delegated_human_responses WHERE target_id = ? AND consumed_at <> ''
		ORDER BY consumed_at DESC LIMIT 1`), targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return DelegatedResponse{}, ErrDelegatedResponseNotFound
	}
	return record, err
}
