// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
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
	var compressed []byte
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT compressed_content FROM task_message_payloads WHERE digest = ?
	`), message.PayloadDigest).Scan(&compressed)
	if err != nil {
		return fmt.Errorf("failed to load message payload %s: %w", message.PayloadDigest, err)
	}
	payloadBytes, err := gzipDecompress(compressed)
	if err != nil {
		return fmt.Errorf("failed to decompress message payload %s: %w", message.PayloadDigest, err)
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

func gzipDecompress(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: %w", err)
	}
	defer func() { _ = gr.Close() }()
	out, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("gzip decompress: read: %w", err)
	}
	return out, nil
}
