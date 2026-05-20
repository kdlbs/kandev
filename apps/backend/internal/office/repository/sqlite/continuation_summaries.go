package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// continuationSummaryMaxBytes caps the markdown body stored in
// agent_continuation_summaries.content. The spec mandates 8 KB; bytes
// (not runes) are the relevant budget because the prompt slice that
// ends up in front of the model is also a byte slice. Truncation
// happens at the byte boundary; multi-byte runes that fall on the
// boundary are dropped rather than corrupted.
const continuationSummaryMaxBytes = 8192

// AgentContinuationSummary is one row in agent_continuation_summaries —
// the per-(agent, scope) markdown blob that bridges context across
// fresh-session runs (heartbeats, lightweight routines).
type AgentContinuationSummary struct {
	AgentProfileID string    `db:"agent_profile_id" json:"agent_profile_id"`
	Scope          string    `db:"scope" json:"scope"`
	Content        string    `db:"content" json:"content"`
	ContentTokens  int       `db:"content_tokens" json:"content_tokens"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
	UpdatedByRunID string    `db:"updated_by_run_id" json:"updated_by_run_id"`
}

// GetContinuationSummary returns the row for (agent, scope). Returns
// nil + sql.ErrNoRows when no row exists; callers should treat that as
// "no prior summary" rather than an error.
func (r *Repository) GetContinuationSummary(
	ctx context.Context, agentProfileID, scope string,
) (*AgentContinuationSummary, error) {
	var row AgentContinuationSummary
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT agent_profile_id, scope, content, content_tokens,
		       updated_at, updated_by_run_id
		FROM agent_continuation_summaries
		WHERE agent_profile_id = ? AND scope = ?
	`), agentProfileID, scope).StructScan(&row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &row, nil
}

// UpsertContinuationSummary writes the row using SQLite's
// ON CONFLICT (agent_profile_id, scope) DO UPDATE upsert. The
// Content field is truncated to 8 KB before write — callers do not
// need to pre-truncate. UpdatedAt is stamped to time.Now().UTC() if
// the caller passed a zero value.
//
// Truncation note: cuts at byte boundary (8192 bytes). Multi-byte
// UTF-8 runes that straddle the boundary are dropped to keep the
// stored string valid UTF-8.
func (r *Repository) UpsertContinuationSummary(
	ctx context.Context, row AgentContinuationSummary,
) error {
	row.Content = truncateUTF8(row.Content, continuationSummaryMaxBytes)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_continuation_summaries (
			agent_profile_id, scope, content, content_tokens,
			updated_at, updated_by_run_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (agent_profile_id, scope) DO UPDATE SET
			content = excluded.content,
			content_tokens = excluded.content_tokens,
			updated_at = excluded.updated_at,
			updated_by_run_id = excluded.updated_by_run_id
	`), row.AgentProfileID, row.Scope, row.Content, row.ContentTokens,
		row.UpdatedAt, row.UpdatedByRunID)
	return err
}

// truncateUTF8 returns s truncated to at most maxBytes, cutting at a
// rune boundary. When the cut would split a multi-byte rune the
// straddling bytes are dropped.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to the start of the last fully-included
	// rune. UTF-8 continuation bytes match 10xxxxxx (0x80..0xBF); the
	// loop terminates as soon as we land on a non-continuation byte.
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}
