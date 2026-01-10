package protocol

// NewProgressMessage creates a new progress message
func NewProgressMessage(agentID, taskID string, data ProgressData) *Message {
	return NewMessage(MessageTypeProgress, agentID, taskID, map[string]interface{}{
		"progress":        data.Progress,
		"message":         data.Message,
		"current_file":    data.CurrentFile,
		"files_processed": data.FilesProcessed,
		"total_files":     data.TotalFiles,
	})
}

// NewLogMessage creates a new log message
func NewLogMessage(agentID, taskID string, data LogData) *Message {
	return NewMessage(MessageTypeLog, agentID, taskID, map[string]interface{}{
		"level":    data.Level,
		"message":  data.Message,
		"metadata": data.Metadata,
	})
}

// NewResultMessage creates a new result message
func NewResultMessage(agentID, taskID string, data ResultData) *Message {
	// Convert artifacts to interface slice
	artifacts := make([]interface{}, len(data.Artifacts))
	for i, a := range data.Artifacts {
		artifacts[i] = map[string]interface{}{
			"type": a.Type,
			"path": a.Path,
			"url":  a.URL,
		}
	}

	return NewMessage(MessageTypeResult, agentID, taskID, map[string]interface{}{
		"status":    data.Status,
		"summary":   data.Summary,
		"artifacts": artifacts,
	})
}

// NewErrorMessage creates a new error message
func NewErrorMessage(agentID, taskID string, data ErrorData) *Message {
	return NewMessage(MessageTypeError, agentID, taskID, map[string]interface{}{
		"error":   data.Error,
		"file":    data.File,
		"details": data.Details,
	})
}

// NewStatusMessage creates a new status message
func NewStatusMessage(agentID, taskID string, data StatusData) *Message {
	return NewMessage(MessageTypeStatus, agentID, taskID, map[string]interface{}{
		"status":  data.Status,
		"message": data.Message,
	})
}

// NewHeartbeatMessage creates a new heartbeat message
func NewHeartbeatMessage(agentID, taskID string) *Message {
	return NewMessage(MessageTypeHeartbeat, agentID, taskID, map[string]interface{}{})
}

// NewControlMessage creates a new control message
func NewControlMessage(agentID, taskID string, data ControlData) *Message {
	return NewMessage(MessageTypeControl, agentID, taskID, map[string]interface{}{
		"action": data.Action,
		"reason": data.Reason,
	})
}

