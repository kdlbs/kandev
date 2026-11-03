package messagequeue

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Recovery evidence preserves JSON numbers exactly. The ordinary delivery
// scanner remains unchanged: these records never enter a delivery path.
func scanRecoveryRow(scanner interface{ Scan(...interface{}) error }) (*QueuedMessage, error) {
	entry := &QueuedMessage{}
	var planMode int
	var attachments, metadata string
	if err := scanner.Scan(&entry.ID, &entry.SessionID, &entry.TaskID, &entry.Position,
		&entry.Content, &entry.Model, &planMode, &attachments, &metadata, &entry.QueuedAt, &entry.QueuedBy); err != nil {
		return nil, err
	}
	entry.PlanMode = planMode != 0
	if attachments != "" && attachments != "[]" {
		if err := json.Unmarshal([]byte(attachments), &entry.Attachments); err != nil {
			return nil, err
		}
	}
	if metadata != "" && metadata != "{}" {
		decoder := json.NewDecoder(strings.NewReader(metadata))
		decoder.UseNumber()
		if err := decoder.Decode(&entry.Metadata); err != nil {
			return nil, err
		}
		if err := decoder.Decode(new(interface{})); err != io.EOF {
			return nil, errors.New("invalid recovery metadata")
		}
	}
	return entry, nil
}
