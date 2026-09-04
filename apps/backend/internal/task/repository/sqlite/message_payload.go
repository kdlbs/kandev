// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// largeMessagePayloadThresholdBytes is the combined stdout+stderr size above
// which a shell command's output is externalized into task_message_payloads
// instead of staying inline in task_session_messages.metadata. Chosen to keep
// the overwhelming majority of tool calls (short commands, status checks,
// small diffs) fully inline while bounding the handful of large build/test
// logs that otherwise dominate SQLite growth.
const largeMessagePayloadThresholdBytes = 4096

// maxMessagePayloadRehydrateBytes caps one explicit shell-output expansion.
// The writer normally receives already-bounded agentctl output, but the read
// path must also defend against corrupt or hostile local database rows.
const maxMessagePayloadRehydrateBytes = 64 * 1024 * 1024

// externalizeMessagePayload computes the metadata JSON CreateMessage/
// UpdateMessage should persist for a message, externalizing a large shell
// command output snapshot into task_message_payloads (content-addressed by
// SHA-256 digest, so byte-identical output across messages naturally
// deduplicates) and replacing it with the same small summary
// ProjectMessageMetadata already computes for client responses. Small
// metadata (the common case) is marshaled unchanged. On success, message's
// PayloadDigest/PayloadSize are set to reflect what was persisted (empty/zero
// when nothing was externalized).
//
// The payload row is written before this function returns so a concurrent or
// later message insert referencing its digest can never observe a dangling
// reference; content-addressing makes writing it more than once (a
// duplicate output from a different message) an idempotent no-op.
func (r *Repository) externalizeMessagePayload(ctx context.Context, message *models.Message) (string, error) {
	message.PayloadDigest = ""
	message.PayloadSize = 0
	if message.Metadata == nil {
		return "{}", nil
	}

	output, ok := models.ExtractShellExecOutput(message.Metadata)
	if !ok || len(output.Stdout)+len(output.Stderr) <= largeMessagePayloadThresholdBytes {
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to serialize message metadata: %w", err)
		}
		return string(metadataBytes), nil
	}

	payloadBytes, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to serialize shell output payload: %w", err)
	}
	digest := sha256Hex(payloadBytes)
	if err := r.putMessagePayload(ctx, digest, payloadBytes); err != nil {
		return "", fmt.Errorf("failed to store externalized message payload: %w", err)
	}

	projected := models.ProjectMessageMetadata(message.Metadata)
	metadataBytes, err := json.Marshal(projected)
	if err != nil {
		return "", fmt.Errorf("failed to serialize projected message metadata: %w", err)
	}
	message.PayloadDigest = digest
	message.PayloadSize = int64(len(payloadBytes))
	return string(metadataBytes), nil
}

// putMessagePayload gzip-compresses payload and upserts it into
// task_message_payloads keyed by its digest. ON CONFLICT DO NOTHING makes a
// repeat write of the same content (a duplicate output from another
// message, or a retry) a no-op rather than an error.
func (r *Repository) putMessagePayload(ctx context.Context, digest string, payload []byte) error {
	compressed, err := gzipCompress(payload)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_message_payloads (digest, compressed_content, uncompressed_size, compressed_size, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (digest) DO NOTHING
	`), digest, compressed, len(payload), len(compressed), time.Now().UTC())
	return err
}

// RehydrateMessagePayload resolves message's externalized payload (if any)
// and merges the restored shell output snapshot back into message.Metadata,
// verifying the stored digest against the decompressed content before
// trusting it. A no-op (nil error) when message.PayloadDigest is empty -
// the common case, since only large shell outputs are ever externalized.
// This is the explicit, authorized single-message detail load: callers must
// already have resolved and authorized message (see httpGetShellOutput,
// which fetches by ID under the requesting session).
func (r *Repository) RehydrateMessagePayload(ctx context.Context, message *models.Message) error {
	if message == nil || message.PayloadDigest == "" {
		return nil
	}
	var uncompressedSize, compressedSize, actualCompressedSize int64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT uncompressed_size, compressed_size, length(compressed_content) FROM task_message_payloads WHERE digest = ?
	`), message.PayloadDigest).Scan(&uncompressedSize, &compressedSize, &actualCompressedSize)
	if err != nil {
		return fmt.Errorf("failed to load message payload %s: %w", message.PayloadDigest, err)
	}
	if uncompressedSize < 0 {
		return fmt.Errorf("message payload %s has invalid rehydrate size: %d",
			message.PayloadDigest, uncompressedSize)
	}
	if uncompressedSize > maxMessagePayloadRehydrateBytes {
		return fmt.Errorf("message payload %s exceeds maximum rehydrate size: %d > %d",
			message.PayloadDigest, uncompressedSize, maxMessagePayloadRehydrateBytes)
	}
	if compressedSize < 0 {
		return fmt.Errorf("message payload %s has invalid compressed size: %d",
			message.PayloadDigest, compressedSize)
	}
	if compressedSize > maxMessagePayloadRehydrateBytes {
		return fmt.Errorf("message payload %s exceeds maximum compressed size: %d > %d",
			message.PayloadDigest, compressedSize, maxMessagePayloadRehydrateBytes)
	}
	if actualCompressedSize != compressedSize {
		return fmt.Errorf("message payload %s compressed size mismatch: %d != %d",
			message.PayloadDigest, actualCompressedSize, compressedSize)
	}

	var compressed []byte
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT compressed_content FROM task_message_payloads WHERE digest = ?
	`), message.PayloadDigest).Scan(&compressed)
	if err != nil {
		return fmt.Errorf("failed to load message payload %s: %w", message.PayloadDigest, err)
	}
	payloadBytes, err := gzipDecompressExpectedSize(compressed, maxMessagePayloadRehydrateBytes, uncompressedSize)
	if err != nil {
		return fmt.Errorf("failed to decompress message payload %s: %w", message.PayloadDigest, err)
	}
	if int64(len(payloadBytes)) != uncompressedSize {
		return fmt.Errorf("message payload %s size mismatch: %d != %d",
			message.PayloadDigest, len(payloadBytes), uncompressedSize)
	}
	if actual := sha256Hex(payloadBytes); actual != message.PayloadDigest {
		return fmt.Errorf("message payload %s failed integrity check (got %s)", message.PayloadDigest, actual)
	}
	var output models.ShellExecOutputSnapshot
	if err := json.Unmarshal(payloadBytes, &output); err != nil {
		return fmt.Errorf("failed to deserialize message payload %s: %w", message.PayloadDigest, err)
	}
	if rehydrated, ok := models.RehydrateShellOutput(message.Metadata, output); ok {
		message.Metadata = rehydrated
	}
	return nil
}

// OrphanedMessagePayloadCandidate identifies a task_message_payloads row no
// longer referenced by any message's payload_digest. Content-addressing
// means a row can be shared by many messages; it only becomes eligible once
// every referencing message has been deleted or edited away from it.
type OrphanedMessagePayloadCandidate struct {
	Digest           string
	CompressedSize   int64
	UncompressedSize int64
}

// ListOrphanedMessagePayloadCandidates returns task_message_payloads rows no
// task_session_messages row currently references. Read-only and
// non-destructive - reported for a later, explicit maintenance command
// (Task 06) to act on, mirroring ListDuplicateGitSnapshotCandidates and
// ListObsoletePlanRevisionCandidates.
func (r *Repository) ListOrphanedMessagePayloadCandidates(ctx context.Context, limit int) ([]OrphanedMessagePayloadCandidate, error) {
	query := `
		SELECT p.digest, p.compressed_size, p.uncompressed_size
		FROM task_message_payloads p
		WHERE NOT EXISTS (
			SELECT 1 FROM task_session_messages m WHERE m.payload_digest = p.digest
		)
		ORDER BY p.digest ASC`
	return listLimitedCandidates(ctx, r.ro, query, limit, "orphaned message payload candidates",
		func(rows *sql.Rows) (OrphanedMessagePayloadCandidate, error) {
			var c OrphanedMessagePayloadCandidate
			err := rows.Scan(&c.Digest, &c.CompressedSize, &c.UncompressedSize)
			return c, err
		})
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		_ = gw.Close()
		return nil, fmt.Errorf("gzip compress: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("gzip compress: close: %w", err)
	}
	return buf.Bytes(), nil
}

func gzipDecompress(data []byte, maxBytes int64) ([]byte, error) {
	return gzipDecompressExpectedSize(data, maxBytes, -1)
}

func gzipDecompressExpectedSize(data []byte, maxBytes, expectedSize int64) ([]byte, error) {
	if maxBytes < 0 || expectedSize < -1 {
		return nil, fmt.Errorf("gzip decompress: invalid size limit")
	}
	if expectedSize > maxBytes {
		return nil, fmt.Errorf("gzip decompress: expected size %d exceeds maximum size %d", expectedSize, maxBytes)
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	defer func() { _ = gr.Close() }()
	readLimit := maxBytes + 1
	if expectedSize >= 0 && expectedSize < readLimit {
		readLimit = expectedSize + 1
	}
	out, err := io.ReadAll(io.LimitReader(gr, readLimit))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: read: %w", err)
	}
	if int64(len(out)) > maxBytes {
		return nil, fmt.Errorf("gzip decompress: payload exceeds maximum size %d", maxBytes)
	}
	if expectedSize >= 0 && int64(len(out)) != expectedSize {
		return nil, fmt.Errorf("gzip decompress: payload size mismatch: %d != %d", len(out), expectedSize)
	}
	return out, nil
}
