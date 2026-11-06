package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// QueueDispositionStatus is the durable, per-entry result vocabulary returned
// by exact queue disposition.
type QueueDispositionStatus string

const (
	QueueDispositionRemoved  QueueDispositionStatus = "removed"
	QueueDispositionNotFound QueueDispositionStatus = "not_found"
	QueueDispositionChanged  QueueDispositionStatus = "changed"
)

// ErrInvalidQueueDisposition identifies malformed exact-disposition input.
// Repository and transaction errors are deliberately not classified as
// validation failures at the transport boundary.
var ErrInvalidQueueDisposition = errors.New("invalid queue disposition")

// QueueEntryClaim binds an immutable queue entry ID to the exact snapshot an
// authorized caller observed in a census. Claims are opaque to callers.
type QueueEntryClaim struct {
	ID    string `json:"id"`
	Claim string `json:"claim"`
}

// QueueDispositionOutcome reports whether one claimed entry was removed.
type QueueDispositionOutcome struct {
	ID     string                 `json:"id"`
	Status QueueDispositionStatus `json:"status"`
}

// QueueDispositionResult reports atomic before/after counts and every requested
// entry outcome. Counts include visible pending entries only.
type QueueDispositionResult struct {
	BeforeCount int                       `json:"before_count"`
	AfterCount  int                       `json:"after_count"`
	Outcomes    []QueueDispositionOutcome `json:"outcomes"`
}

// QueueCensusEntry is a content-free descriptor for one visible pending FIFO
// entry. The digest permits equality checks without exposing the message body.
type QueueCensusEntry struct {
	ID              string `json:"id"`
	Claim           string `json:"claim"`
	Position        int64  `json:"position"`
	QueuedAt        string `json:"queued_at"`
	QueuedBy        string `json:"queued_by"`
	Origin          string `json:"origin,omitempty"`
	SenderTaskID    string `json:"sender_task_id,omitempty"`
	RoutineWake     bool   `json:"routine_wake"`
	RoutineIdentity string `json:"routine_identity,omitempty"`
	ContentSHA256   string `json:"content_sha256"`
	ContentBytes    int    `json:"content_bytes"`
	AttachmentCount int    `json:"attachment_count"`
}

// QueueCensus is the exact FIFO snapshot returned to a session-bound caller.
type QueueCensus struct {
	Entries     []QueueCensusEntry `json:"entries"`
	BeforeCount int                `json:"before_count"`
	Max         int                `json:"max"`
	AutoRun     bool               `json:"auto_run"`
}

// Census returns a content-free FIFO snapshot for one session.
func (s *Service) Census(ctx context.Context, sessionID string) (*QueueCensus, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list queue census: %w", err)
	}
	autoRun, err := s.repo.GetAutoRun(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read queue auto-run: %w", err)
	}
	result := &QueueCensus{
		Entries: make([]QueueCensusEntry, 0, len(entries)),
		Max:     s.MaxPerSession(),
		AutoRun: autoRun,
	}
	for i := range entries {
		entry := &entries[i]
		if entry.IsReservedInFlight() {
			continue
		}
		contentDigest := sha256.Sum256([]byte(entry.Content))
		result.Entries = append(result.Entries, QueueCensusEntry{
			ID:              entry.ID,
			Claim:           queueSnapshotClaim(entry),
			Position:        entry.Position,
			QueuedAt:        entry.QueuedAt.UTC().Format(time.RFC3339Nano),
			QueuedBy:        entry.QueuedBy,
			Origin:          metadataString(entry.Metadata, "origin"),
			SenderTaskID:    metadataString(entry.Metadata, MetadataSenderTaskID),
			RoutineWake:     metadataBool(entry.Metadata, MetadataRoutineWake),
			RoutineIdentity: metadataString(entry.Metadata, MetadataRoutineIdentity),
			ContentSHA256:   hex.EncodeToString(contentDigest[:]),
			ContentBytes:    len(entry.Content),
			AttachmentCount: len(entry.Attachments),
		})
	}
	result.BeforeCount = len(result.Entries)
	return result, nil
}

func metadataBool(metadata map[string]interface{}, key string) bool {
	value, _ := metadata[key].(bool)
	return value
}

// DisposeExact atomically removes only unchanged exact entries. Empty,
// duplicate, or malformed claims are rejected before repository mutation.
func (s *Service) DisposeExact(ctx context.Context, sessionID string, claims []QueueEntryClaim) (*QueueDispositionResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session id is required", ErrInvalidQueueDisposition)
	}
	if len(claims) == 0 {
		return nil, fmt.Errorf("%w: at least one queue entry claim is required", ErrInvalidQueueDisposition)
	}
	seen := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if claim.ID == "" || claim.Claim == "" {
			return nil, fmt.Errorf("%w: queue entry id and claim are required", ErrInvalidQueueDisposition)
		}
		if _, exists := seen[claim.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate queue entry id: %s", ErrInvalidQueueDisposition, claim.ID)
		}
		seen[claim.ID] = struct{}{}
	}
	result, err := s.repo.DisposeExact(ctx, sessionID, claims)
	if err != nil {
		return nil, err
	}
	s.logger.Info("exact queue disposition completed",
		zap.String("session_id", sessionID),
		zap.Int("before_count", result.BeforeCount),
		zap.Int("after_count", result.AfterCount),
		zap.Any("outcomes", result.Outcomes))
	return result, nil
}

func queueSnapshotClaim(entry *QueuedMessage) string {
	if entry == nil {
		return ""
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func visibleQueueCount(entries []*QueuedMessage) int {
	count := 0
	for _, entry := range entries {
		if entry != nil && !entry.IsReservedInFlight() {
			count++
		}
	}
	return count
}
