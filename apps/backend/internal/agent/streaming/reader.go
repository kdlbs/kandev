// Package streaming provides ACP message streaming from agent containers.
package streaming

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/acp/protocol"
)

// StreamReader reads ACP messages from a container's output
type StreamReader struct {
	instanceID string
	taskID     string
	eventBus   bus.EventBus
	logger     *logger.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

// NewStreamReader creates a new stream reader
func NewStreamReader(
	instanceID string,
	taskID string,
	eventBus bus.EventBus,
	log *logger.Logger,
) *StreamReader {
	return &StreamReader{
		instanceID: instanceID,
		taskID:     taskID,
		eventBus:   eventBus,
		logger: log.WithFields(
			zap.String("component", "stream-reader"),
			zap.String("instance_id", instanceID),
			zap.String("task_id", taskID),
		),
	}
}

// Start begins reading from the given reader (container logs)
func (r *StreamReader) Start(ctx context.Context, reader io.ReadCloser) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil // Already running
	}
	r.running = true

	// Create a cancellable context
	ctx, r.cancel = context.WithCancel(ctx)
	r.mu.Unlock()

	r.logger.Info("starting stream reader")

	r.wg.Add(1)
	go r.readLoop(ctx, reader)

	return nil
}

// Stop stops the stream reader
func (r *StreamReader) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}

	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()

	r.wg.Wait()

	r.mu.Lock()
	r.running = false
	r.mu.Unlock()

	r.logger.Info("stream reader stopped")
	return nil
}

// IsRunning returns true if the reader is active
func (r *StreamReader) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// readLoop reads lines from the reader and parses ACP messages
func (r *StreamReader) readLoop(ctx context.Context, reader io.ReadCloser) {
	defer r.wg.Done()
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	// Increase buffer size for potentially large JSON messages
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for {
		select {
		case <-ctx.Done():
			r.logger.Debug("read loop cancelled")
			return
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					r.logger.Error("scanner error", zap.Error(err))
				} else {
					r.logger.Debug("EOF reached")
				}
				return
			}

			line := scanner.Text()
			r.processLine(ctx, line)
		}
	}
}

// processLine processes a single line of output
func (r *StreamReader) processLine(ctx context.Context, line string) {
	// Skip empty lines
	if len(line) == 0 {
		return
	}

	// Handle Docker multiplexed output prefix (8 bytes header)
	// Docker log format: [8-byte header][payload]
	// The header contains stream type and size
	cleanedLine := r.stripDockerHeader(line)
	if len(cleanedLine) == 0 {
		return
	}

	// Try to parse as JSON ACP message
	msg, err := protocol.Parse([]byte(cleanedLine))
	if err != nil {
		// Not valid JSON, treat as regular log line
		r.logger.Debug("non-ACP output", zap.String("line", cleanedLine))
		return
	}

	// Check if it's a valid ACP message
	if !msg.IsValid() {
		r.logger.Debug("invalid ACP message (missing required fields)",
			zap.String("line", cleanedLine))
		return
	}

	// Publish the message
	if err := r.publishMessage(ctx, msg); err != nil {
		r.logger.Error("failed to publish ACP message",
			zap.String("message_type", string(msg.Type)),
			zap.Error(err))
	}
}

// stripDockerHeader removes Docker multiplexed output header and timestamps if present
// Docker log format: [8-byte header][payload] or [timestamp][space][payload]
// Header format: [stream_type(1)][0(3)][size(4)]
// Timestamp format: 2026-01-10T00:42:29.114697835Z {json...}
func (r *StreamReader) stripDockerHeader(line string) string {
	// Check if line starts with a Docker stream header
	// Stream types: 0=stdin, 1=stdout, 2=stderr
	if len(line) < 8 {
		return line
	}

	// Check if first byte is a valid stream type (0, 1, or 2)
	// and next 3 bytes are zeros
	if (line[0] == 0 || line[0] == 1 || line[0] == 2) &&
		line[1] == 0 && line[2] == 0 && line[3] == 0 {
		// This looks like a Docker header, skip first 8 bytes
		return line[8:]
	}

	// Handle Docker timestamp format: "2026-01-10T00:42:29.114697835Z {...}"
	// The timestamp is typically 30-35 characters followed by a space and JSON
	// Look for JSON start anywhere in the line (for timestamp-prefixed logs)
	if idx := strings.Index(line, "{"); idx > 0 {
		return line[idx:]
	}

	return line
}

// publishMessage publishes an ACP message to NATS
func (r *StreamReader) publishMessage(ctx context.Context, msg *protocol.Message) error {
	if r.eventBus == nil {
		return nil
	}

	// Build the event data from the ACP message
	data := map[string]interface{}{
		"type":       string(msg.Type),
		"timestamp":  msg.Timestamp,
		"agent_id":   msg.AgentID,
		"task_id":    msg.TaskID,
		"data":       msg.Data,
	}

	// Create and publish event
	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(r.taskID)

	if err := r.eventBus.Publish(ctx, subject, event); err != nil {
		return err
	}

	r.logger.Debug("published ACP message",
		zap.String("subject", subject),
		zap.String("message_type", string(msg.Type)))

	return nil
}

// marshalMessage marshals an ACP message for logging/debugging
func (r *StreamReader) marshalMessage(msg *protocol.Message) string {
	data, err := json.Marshal(msg)
	if err != nil {
		return ""
	}
	return string(data)
}

