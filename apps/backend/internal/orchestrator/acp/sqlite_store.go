package acp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/pkg/acp/protocol"
)

// SQLiteMessageStore is a SQLite-based implementation of MessageStore for persistent log storage
type SQLiteMessageStore struct {
	db *sql.DB
}

// NewSQLiteMessageStore creates a new SQLite message store using an existing database connection
func NewSQLiteMessageStore(db *sql.DB) *SQLiteMessageStore {
	return &SQLiteMessageStore{db: db}
}

// Store saves an ACP message to the SQLite database
func (s *SQLiteMessageStore) Store(ctx context.Context, msg *protocol.Message) error {
	id := uuid.New().String()

	// Serialize the Data map to JSON for metadata storage
	metadata, err := json.Marshal(msg.Data)
	if err != nil {
		metadata = []byte("{}")
	}

	// Extract log level from message type/data
	logLevel := s.extractLogLevel(msg)

	// Extract message text from the data
	messageText := s.extractMessageText(msg)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO task_agent_execution_logs
		(id, task_id, agent_instance_id, log_level, message_type, message, metadata, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, msg.TaskID, msg.AgentID, logLevel, string(msg.Type), messageText, string(metadata), msg.Timestamp)

	return err
}

// GetMessages retrieves messages for a task
func (s *SQLiteMessageStore) GetMessages(ctx context.Context, taskID string, limit int, since time.Time) ([]*protocol.Message, error) {
	var rows *sql.Rows
	var err error

	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT task_id, agent_instance_id, message_type, metadata, timestamp
			FROM task_agent_execution_logs
			WHERE task_id = ? AND timestamp > ?
			ORDER BY timestamp ASC
			LIMIT ?
		`, taskID, since, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT task_id, agent_instance_id, message_type, metadata, timestamp
			FROM task_agent_execution_logs
			WHERE task_id = ? AND timestamp > ?
			ORDER BY timestamp ASC
		`, taskID, since)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// GetAllMessages retrieves all messages for a task (for historical replay)
func (s *SQLiteMessageStore) GetAllMessages(ctx context.Context, taskID string) ([]*protocol.Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_id, agent_instance_id, message_type, metadata, timestamp
		FROM task_agent_execution_logs
		WHERE task_id = ?
		ORDER BY timestamp ASC
	`, taskID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// GetLatestProgress retrieves the most recent progress for a task
func (s *SQLiteMessageStore) GetLatestProgress(ctx context.Context, taskID string) (*protocol.ProgressData, error) {
	var metadataStr string
	var messageType string

	err := s.db.QueryRowContext(ctx, `
		SELECT message_type, metadata
		FROM task_agent_execution_logs
		WHERE task_id = ? AND message_type = 'progress'
		ORDER BY timestamp DESC
		LIMIT 1
	`, taskID).Scan(&messageType, &metadataStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse metadata to extract progress data
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &data); err != nil {
		return nil, nil
	}

	return s.extractProgressDataFromMap(data), nil
}

// Delete removes all messages for a task
func (s *SQLiteMessageStore) Delete(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM task_agent_execution_logs WHERE task_id = ?`, taskID)
	return err
}

// scanMessages scans database rows into protocol.Message slices
func (s *SQLiteMessageStore) scanMessages(rows *sql.Rows) ([]*protocol.Message, error) {
	var result []*protocol.Message

	for rows.Next() {
		var taskID, agentID, messageType, metadataStr string
		var timestamp time.Time

		err := rows.Scan(&taskID, &agentID, &messageType, &metadataStr, &timestamp)
		if err != nil {
			return nil, err
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &data); err != nil {
			data = make(map[string]interface{})
		}

		msg := &protocol.Message{
			Type:      protocol.MessageType(messageType),
			Timestamp: timestamp,
			AgentID:   agentID,
			TaskID:    taskID,
			Data:      data,
		}
		result = append(result, msg)
	}

	return result, rows.Err()
}

// extractLogLevel extracts a log level from the ACP message
func (s *SQLiteMessageStore) extractLogLevel(msg *protocol.Message) string {
	// For log messages, extract level from data
	if msg.Type == protocol.MessageTypeLog {
		if level, ok := msg.Data["level"].(string); ok {
			return level
		}
	}

	// Map message types to log levels
	switch msg.Type {
	case protocol.MessageTypeError:
		return "error"
	case protocol.MessageTypeProgress, protocol.MessageTypeStatus:
		return "info"
	case protocol.MessageTypeHeartbeat:
		return "debug"
	default:
		return "info"
	}
}

// extractMessageText extracts a human-readable message from the ACP message
func (s *SQLiteMessageStore) extractMessageText(msg *protocol.Message) string {
	// Try common message fields
	if message, ok := msg.Data["message"].(string); ok {
		return message
	}
	if text, ok := msg.Data["text"].(string); ok {
		return text
	}
	if errMsg, ok := msg.Data["error"].(string); ok {
		return errMsg
	}
	if summary, ok := msg.Data["summary"].(string); ok {
		return summary
	}

	// For content updates
	if msg.Type == protocol.MessageTypeProgress {
		if progress, ok := msg.Data["progress"].(float64); ok {
			return fmt.Sprintf("%d%% complete", int(progress))
		}
	}

	return ""
}

// extractProgressDataFromMap extracts ProgressData from a map
func (s *SQLiteMessageStore) extractProgressDataFromMap(data map[string]interface{}) *protocol.ProgressData {
	pd := &protocol.ProgressData{}

	if progress, ok := data["progress"].(float64); ok {
		pd.Progress = int(progress)
	} else if progress, ok := data["progress"].(int); ok {
		pd.Progress = progress
	}

	if message, ok := data["message"].(string); ok {
		pd.Message = message
	}

	if currentFile, ok := data["current_file"].(string); ok {
		pd.CurrentFile = currentFile
	}

	if filesProcessed, ok := data["files_processed"].(float64); ok {
		pd.FilesProcessed = int(filesProcessed)
	} else if filesProcessed, ok := data["files_processed"].(int); ok {
		pd.FilesProcessed = filesProcessed
	}

	if totalFiles, ok := data["total_files"].(float64); ok {
		pd.TotalFiles = int(totalFiles)
	} else if totalFiles, ok := data["total_files"].(int); ok {
		pd.TotalFiles = totalFiles
	}

	return pd
}

