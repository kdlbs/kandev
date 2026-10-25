package messagequeue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	taskcontract "github.com/kandev/kandev/internal/task/contract"
	"io"
	"time"
	"unicode/utf8"
)

const (
	DefaultRecoveryBodyBytes = 65536
	MinRecoveryBodyBytes     = 1024
	MaxRecoveryBodyBytes     = 262144
	MaxRecoveryResponseBytes = taskcontract.MaxQueueRecoveryResponseBytes
	MaxRecoveryFragments     = 256
	MaxRecoveryCursorBytes   = 4096
)

var (
	ErrInvalidRecoveryPage = errors.New("invalid queue recovery page")
	ErrRecoveryChanged     = errors.New("queue recovery snapshot changed")
)

type QueueRecoveryEntry struct {
	ID               string `json:"id"`
	Position         int64  `json:"position"`
	ContentSHA256    string `json:"content_sha256"`
	ContentBytes     int    `json:"content_bytes"`
	AttachmentCount  int    `json:"attachment_count"`
	EvidenceSHA256   string `json:"evidence_sha256"`
	ReservedInFlight bool   `json:"reserved_in_flight"`
}

type QueueRecoveryManifest struct {
	Version           int                  `json:"version"`
	Identity          QueueSessionIdentity `json:"identity"`
	WorkspaceID       string               `json:"workspace_id"`
	SnapshotSHA256    string               `json:"snapshot_sha256"`
	TotalEntries      int                  `json:"total_entries"`
	TotalContentBytes int                  `json:"total_content_bytes"`
	Entries           []QueueRecoveryEntry `json:"entries"`
}

type QueueRecoveryFragment struct {
	QueueRecoveryEntry
	Body            string `json:"body"`
	BodyOffsetBytes int    `json:"body_offset_bytes"`
	BodyBytes       int    `json:"body_bytes"`
	BodyComplete    bool   `json:"body_complete"`
}

type QueueRecoveryPage struct {
	Version           int                     `json:"version"`
	Identity          QueueSessionIdentity    `json:"identity"`
	WorkspaceID       string                  `json:"workspace_id"`
	SnapshotSHA256    string                  `json:"snapshot_sha256"`
	TotalEntries      int                     `json:"total_entries"`
	TotalContentBytes int                     `json:"total_content_bytes"`
	PageBodyBytes     int                     `json:"page_body_bytes"`
	Fragments         []QueueRecoveryFragment `json:"fragments"`
	NextCursor        string                  `json:"next_cursor,omitempty"`
}

type recoveryCursor struct {
	Version     int                  `json:"version"`
	Identity    QueueSessionIdentity `json:"identity"`
	WorkspaceID string               `json:"workspace_id"`
	Digest      string               `json:"digest"`
	Index       int                  `json:"index"`
	Offset      int                  `json:"offset"`
}

func (s *Service) RecoverySnapshotManifest(ctx context.Context, identity QueueSessionIdentity, workspaceID string) (*QueueRecoveryManifest, error) {
	snapshot, err := s.RecoveryRead(ctx, identity)
	if err != nil {
		return nil, err
	}
	return recoveryManifest(identity, workspaceID, snapshot.Entries)
}

func recoveryHash(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func recoveryManifest(identity QueueSessionIdentity, workspaceID string, entries []QueuedMessage) (*QueueRecoveryManifest, error) {
	if workspaceID == "" {
		return nil, ErrInvalidRecoveryPage
	}
	manifest := &QueueRecoveryManifest{Version: 1, Identity: identity, WorkspaceID: workspaceID, TotalEntries: len(entries), Entries: make([]QueueRecoveryEntry, 0, len(entries))}
	var previous int64
	seen := make(map[string]bool, len(entries))
	for i := range entries {
		entry := entries[i]
		if entry.TaskID != identity.TaskID || entry.SessionID != identity.SessionID || entry.ID == "" || seen[entry.ID] || i > 0 && entry.Position <= previous {
			return nil, ErrSessionIdentityMismatch
		}
		if !utf8.ValidString(entry.Content) {
			return nil, ErrInvalidRecoveryPage
		}
		seen[entry.ID], previous = true, entry.Position
		entry.QueuedAt = entry.QueuedAt.UTC().Truncate(time.Microsecond)
		evidence, err := recoveryHash(entry)
		if err != nil {
			return nil, err
		}
		bodyHash := sha256.Sum256([]byte(entry.Content))
		manifest.Entries = append(manifest.Entries, QueueRecoveryEntry{ID: entry.ID, Position: entry.Position, ContentSHA256: hex.EncodeToString(bodyHash[:]), ContentBytes: len(entry.Content), AttachmentCount: len(entry.Attachments), EvidenceSHA256: evidence, ReservedInFlight: entry.IsReservedInFlight()})
		manifest.TotalContentBytes += len(entry.Content)
	}
	digest, err := recoveryHash(manifest)
	if err != nil {
		return nil, err
	}
	manifest.SnapshotSHA256 = digest
	if err := recoveryResponseBound(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func recoveryResponseBound(value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(encoded) > MaxRecoveryResponseBytes {
		return ErrRecoveryLimit
	}
	return nil
}

func decodeRecoveryCursor(raw string, manifest *QueueRecoveryManifest, entries []QueuedMessage) (recoveryCursor, error) {
	start := recoveryCursor{Version: 1, Identity: manifest.Identity, WorkspaceID: manifest.WorkspaceID, Digest: manifest.SnapshotSHA256}
	if raw == "" {
		return start, nil
	}
	if len(raw) > MaxRecoveryCursorBytes {
		return start, ErrInvalidRecoveryPage
	}
	encoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return start, ErrInvalidRecoveryPage
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var cursor recoveryCursor
	if err := decoder.Decode(&cursor); err != nil {
		return start, ErrInvalidRecoveryPage
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return start, ErrInvalidRecoveryPage
	}
	if cursor.Version != 1 || cursor.Identity != start.Identity || cursor.WorkspaceID != start.WorkspaceID || cursor.Index < 0 || cursor.Offset < 0 {
		return start, ErrInvalidRecoveryPage
	}
	if cursor.Digest != start.Digest {
		return start, ErrRecoveryChanged
	}
	if cursor.Index > len(entries) {
		return start, ErrInvalidRecoveryPage
	}
	if err := validateRecoveryOffset(cursor, entries); err != nil {
		return start, err
	}
	return cursor, nil
}

func validateRecoveryOffset(cursor recoveryCursor, entries []QueuedMessage) error {
	if cursor.Index == len(entries) {
		if cursor.Offset != 0 {
			return ErrInvalidRecoveryPage
		}
	} else if cursor.Offset > len(entries[cursor.Index].Content) || !utf8.ValidString(entries[cursor.Index].Content[:cursor.Offset]) {
		return ErrInvalidRecoveryPage
	}
	return nil
}

func (s *Service) RecoverySnapshotPage(ctx context.Context, identity QueueSessionIdentity, workspaceID, rawCursor string, budget int, expected string) (*QueueRecoveryPage, error) {
	if budget == 0 {
		budget = DefaultRecoveryBodyBytes
	}
	if budget < MinRecoveryBodyBytes || budget > MaxRecoveryBodyBytes {
		return nil, ErrInvalidRecoveryPage
	}
	snapshot, err := s.RecoveryRead(ctx, identity)
	if err != nil {
		return nil, err
	}
	manifest, err := recoveryManifest(identity, workspaceID, snapshot.Entries)
	if err != nil {
		return nil, err
	}
	if expected != "" && expected != manifest.SnapshotSHA256 {
		return nil, ErrRecoveryChanged
	}
	cursor, err := decodeRecoveryCursor(rawCursor, manifest, snapshot.Entries)
	if err != nil {
		return nil, err
	}
	page := &QueueRecoveryPage{Version: 1, Identity: identity, WorkspaceID: workspaceID, SnapshotSHA256: manifest.SnapshotSHA256, TotalEntries: manifest.TotalEntries, TotalContentBytes: manifest.TotalContentBytes, Fragments: make([]QueueRecoveryFragment, 0)}
	appendRecoveryFragments(page, &cursor, snapshot.Entries, manifest.Entries, budget)
	if cursor.Index < len(snapshot.Entries) {
		encoded, err := json.Marshal(cursor)
		if err != nil {
			return nil, err
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	if err := recoveryResponseBound(page); err != nil {
		return nil, err
	}
	return page, nil
}

func appendRecoveryFragments(page *QueueRecoveryPage, cursor *recoveryCursor, entries []QueuedMessage, manifest []QueueRecoveryEntry, budget int) {
	for cursor.Index < len(entries) && page.PageBodyBytes < budget && len(page.Fragments) < MaxRecoveryFragments {
		body := entries[cursor.Index].Content
		end := len(body)
		if end-cursor.Offset > budget-page.PageBodyBytes {
			end = cursor.Offset + budget - page.PageBodyBytes
		}
		for end > cursor.Offset && !utf8.ValidString(body[cursor.Offset:end]) {
			end--
		}
		if end == cursor.Offset && end < len(body) {
			break
		}
		fragment := QueueRecoveryFragment{QueueRecoveryEntry: manifest[cursor.Index], Body: body[cursor.Offset:end], BodyOffsetBytes: cursor.Offset, BodyBytes: end - cursor.Offset, BodyComplete: end == len(body)}
		page.Fragments = append(page.Fragments, fragment)
		page.PageBodyBytes += fragment.BodyBytes
		if fragment.BodyComplete {
			cursor.Index++
			cursor.Offset = 0
		} else {
			cursor.Offset = end
		}
	}
}
