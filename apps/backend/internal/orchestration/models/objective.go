package models

import (
	"fmt"
	"strings"
	"time"
)

type Criterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type Evidence struct {
	CriterionID        string `json:"criterion_id"`
	SourceKind         string `json:"source_kind"`
	SourceID           string `json:"source_id"`
	TaskID             string `json:"task_id,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
	AcceptanceRevision int64  `json:"acceptance_revision"`
}

type Objective struct {
	ID                 string      `json:"id" db:"id"`
	BindingID          string      `json:"binding_id" db:"binding_id"`
	WorkspaceID        string      `json:"workspace_id" db:"workspace_id"`
	SourceCommentID    string      `json:"source_comment_id" db:"source_comment_id"`
	Title              string      `json:"title" db:"title"`
	Mode               string      `json:"mode" db:"mode"`
	Status             string      `json:"status" db:"status"`
	Revision           int64       `json:"revision" db:"revision"`
	AcceptanceRevision int64       `json:"acceptance_revision" db:"acceptance_revision"`
	IntentRevision     int64       `json:"intent_revision" db:"intent_revision"`
	Acceptance         []Criterion `json:"acceptance" db:"-"`
	Evidence           []Evidence  `json:"evidence" db:"-"`
	AcceptanceJSON     string      `json:"-" db:"acceptance_json"`
	EvidenceJSON       string      `json:"-" db:"evidence_json"`
	CreatedAt          time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at" db:"updated_at"`
}

type ObjectiveTask struct {
	ObjectiveID string `json:"objective_id" db:"objective_id"`
	TaskID      string `json:"task_id" db:"task_id"`
	SessionID   string `json:"session_id" db:"session_id"`
	Role        string `json:"role" db:"role"`
	ContextRef  string `json:"context_ref" db:"context_ref"`
	OperationID string `json:"operation_id" db:"operation_id"`
}

func (o Objective) Validate() error {
	if strings.TrimSpace(o.Title) == "" || len(o.Title) > 500 || len(o.Acceptance) == 0 || len(o.Acceptance) > 20 || len(o.Evidence) > 40 {
		return fmt.Errorf("objective requires a bounded title and acceptance criteria")
	}
	switch o.Mode {
	case "answer", "inspect", "execute", "design":
	default:
		return fmt.Errorf("unsupported objective mode")
	}
	switch o.Status {
	case "active", "waiting_user", "waiting_dependency", "ready_for_review", "complete", "paused", "cancelled":
	default:
		return fmt.Errorf("unsupported objective status")
	}
	return o.validateEvidenceReferences()
}

func (o Objective) validateEvidenceReferences() error {
	seen := map[string]bool{}
	for _, c := range o.Acceptance {
		if !c.valid() || seen[c.ID] {
			return fmt.Errorf("invalid acceptance criterion")
		}
		seen[c.ID] = true
	}
	for _, e := range o.Evidence {
		if !seen[e.CriterionID] || !e.valid() {
			return fmt.Errorf("invalid evidence reference")
		}
	}
	return nil
}

func (c Criterion) valid() bool {
	return c.ID != "" && len(c.ID) <= 100 && strings.TrimSpace(c.Description) != "" && len(c.Description) <= 1000
}

func (e Evidence) valid() bool {
	return e.SourceID != "" && len(e.SourceID) <= 200 && len(e.TaskID) <= 200 && len(e.SessionID) <= 200 && (e.SourceKind == "comment" || e.SourceKind == "task_message")
}
