package controller

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/models"
)

// SessionTargetPatch distinguishes an omitted request field from an explicit
// null, which clears a previously configured target.
type SessionTargetPatch struct {
	Set    bool
	Target *models.WorkflowSessionTarget
}

func (p *SessionTargetPatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		p.Target = nil
		return nil
	}
	var target models.WorkflowSessionTarget
	if err := json.Unmarshal(data, &target); err != nil {
		return fmt.Errorf("decode session_target: %w", err)
	}
	if err := models.ValidateWorkflowSessionTarget(&target); err != nil {
		return err
	}
	p.Target = &target
	return nil
}
