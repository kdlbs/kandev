package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
)

const (
	fieldTitle    = "title"
	fieldContext  = "context"
	fieldEvidence = mcpcontract.FieldEvidence
	fieldEdits    = "edits"

	improvementTitleMaxRunes     = 60
	improvementRationaleMaxRunes = 10000
	improvementEvidenceMax       = 10
)

// improvementEvidence is one cited run or task; exactly one field is set.
type improvementEvidence struct {
	RunID  string `json:"run_id,omitempty"`
	TaskID string `json:"task_id,omitempty"`
}

// improvementSpec is the stored spec of an improvement proposal.
type improvementSpec struct {
	Title         string                `json:"title"`
	Rationale     string                `json:"rationale"`
	ContextBefore string                `json:"context_before"`
	ContextAfter  string                `json:"context_after"`
	Evidence      []improvementEvidence `json:"evidence"`
}

var improvementTotal = expvar.NewMap("coordinator_improvement_total")

// Closed label set of coordinator_improvement_total.
const (
	improvementProposed      = "proposed"
	improvementApproved      = "approved"
	improvementApplied       = "applied"
	improvementDiscarded     = "discarded"
	improvementApplyConflict = "apply_conflict"
)

func countImprovement(event string) { improvementTotal.Add(event, 1) }

// improvementCounter reads one event counter.
func improvementCounter(event string) int64 {
	if v, ok := improvementTotal.Get(event).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

type improvementKind struct{ svc *Service }

const msgBodyObject = "body must be a JSON object"

func (k *improvementKind) Kind() string             { return ProposalKindImprovement }
func (k *improvementKind) Action() Action           { return ActionImprovement }
func (k *improvementKind) ReRunsOnStaleClaim() bool { return false }

// ValidateEdits refuses every edit: an improvement is approved as proposed.
func (k *improvementKind) ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if len(edits) > 0 {
		if err := json.Unmarshal(edits, &body); err != nil {
			return nil, &FieldError{Field: fieldBody, Message: msgBodyObject}
		}
	}
	if len(body) > 0 {
		return nil, &FieldError{Field: fieldEdits, Message: "an improvement cannot be edited"}
	}
	return base, nil
}

// Execute stores the change the approved improvement describes; the
// coordinator's configuration is not touched.
func (k *improvementKind) Execute(ctx context.Context, claim Claim) (Outcome, error) {
	var spec improvementSpec
	if err := json.Unmarshal(claim.Spec, &spec); err != nil {
		return Outcome{}, failWith("stored improvement spec is unreadable")
	}
	if claim.Coordinator == nil {
		return Outcome{}, failWith("coordinator is unavailable")
	}
	if err := k.svc.store.InsertPendingChange(ctx, claim.Coordinator.ID, claim.ProposalID, spec.ContextBefore, spec.ContextAfter); err != nil {
		return Outcome{}, err
	}
	return Outcome{Detail: truncateRunes(spec.Title, 200)}, nil
}

// ValidatePropose checks every argument that needs no store read and returns
// the spec with context_before still empty. The store-dependent evidence
// checks run in checkImprovementEvidence.
func (k *improvementKind) ValidatePropose(ctx context.Context, c *Coordinator, args json.RawMessage) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(args, &body); err != nil {
		return nil, &FieldError{Field: fieldBody, Message: "arguments are malformed"}
	}
	title, err := improvementText(body, fieldTitle, improvementTitleMaxRunes)
	if err != nil {
		return nil, err
	}
	rationale, err := improvementText(body, fieldRationale, improvementRationaleMaxRunes)
	if err != nil {
		return nil, err
	}
	next, err := improvementContext(body)
	if err != nil {
		return nil, err
	}
	evidence, err := parseImprovementEvidence(body[fieldEvidence])
	if err != nil {
		return nil, err
	}
	if err := k.svc.checkImprovementEvidence(ctx, c, evidence); err != nil {
		return nil, err
	}
	return json.Marshal(improvementSpec{Title: title, Rationale: rationale, ContextAfter: next, Evidence: evidence})
}

// improvementText reads a required string argument, trimmed, of 1 to max runes.
func improvementText(body map[string]json.RawMessage, field string, max int) (string, error) {
	var v string
	if raw, ok := body[field]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", &FieldError{Field: field, Message: field + " must be a string"}
		}
	}
	v = strings.TrimSpace(v)
	if n := utf8.RuneCountInString(v); n < 1 || n > max {
		return "", &FieldError{Field: field, Message: field + " must be 1 to " + strconv.Itoa(max) + " characters"}
	}
	return v, nil
}

// improvementContext reads the replacement context through the phase 1
// context validation; blanking the instructions is a manager's edit.
func improvementContext(body map[string]json.RawMessage) (string, error) {
	var v string
	if raw, ok := body[fieldContext]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", &FieldError{Field: fieldContext, Message: "context must be a string"}
		}
	}
	trimmed, err := ValidateContext(v)
	if err != nil {
		return "", err
	}
	if trimmed == "" {
		return "", &FieldError{Field: fieldContext, Message: "context must not be empty"}
	}
	return trimmed, nil
}

// parseImprovementEvidence shapes the evidence array: 1 to 10 objects, each
// with exactly one non-empty string key run_id or task_id, no repeats, at
// least one run.
func parseImprovementEvidence(raw json.RawMessage) ([]improvementEvidence, error) {
	bad := func(msg string) error { return &FieldError{Field: fieldEvidence, Message: msg} }
	var entries []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &entries) != nil {
		return nil, bad("evidence must be an array")
	}
	if len(entries) < 1 || len(entries) > improvementEvidenceMax {
		return nil, bad("evidence must hold 1 to 10 entries")
	}
	out := make([]improvementEvidence, 0, len(entries))
	seen := map[string]struct{}{}
	hasRun := false
	for _, entry := range entries {
		item, err := parseEvidenceEntry(entry)
		if err != nil {
			return nil, bad(err.Error())
		}
		key := "task:" + item.TaskID
		if item.RunID != "" {
			key, hasRun = "run:"+item.RunID, true
		}
		if _, dup := seen[key]; dup {
			return nil, bad("evidence must not repeat a run or task")
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if !hasRun {
		return nil, bad("evidence must cite at least one run")
	}
	return out, nil
}

var errEvidenceShape = errors.New("each evidence entry must have exactly one of run_id or task_id, a non-empty string")

func parseEvidenceEntry(entry json.RawMessage) (improvementEvidence, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(entry, &obj) != nil || len(obj) != 1 {
		return improvementEvidence{}, errEvidenceShape
	}
	for key, val := range obj {
		var id string
		if json.Unmarshal(val, &id) != nil || strings.TrimSpace(id) == "" {
			return improvementEvidence{}, errEvidenceShape
		}
		id = strings.TrimSpace(id)
		switch key {
		case "run_id":
			return improvementEvidence{RunID: id}, nil
		case "task_id":
			return improvementEvidence{TaskID: id}, nil
		}
	}
	return improvementEvidence{}, errEvidenceShape
}

// checkImprovementEvidence refuses a run that is not this coordinator's and a
// task outside its workspace, with one message for absent and foreign.
func (s *Service) checkImprovementEvidence(ctx context.Context, c *Coordinator, evidence []improvementEvidence) error {
	notFound := &FieldError{Field: fieldEvidence, Message: "evidence names a run or task that was not found"}
	for _, e := range evidence {
		if e.RunID != "" {
			row, err := s.store.turnReadByID(ctx, c.ID, e.RunID)
			if err != nil {
				return err
			}
			if row == nil {
				return notFound
			}
			continue
		}
		if s.kindDeps.Tasks == nil {
			return errors.New("coordinator: task reads are not wired")
		}
		t, err := s.kindDeps.Tasks.GetTarget(ctx, e.TaskID)
		if errors.Is(err, ErrTaskNotFound) {
			return notFound
		}
		if err != nil {
			return err
		}
		if t.WorkspaceID != c.WorkspaceID {
			return notFound
		}
	}
	return nil
}
