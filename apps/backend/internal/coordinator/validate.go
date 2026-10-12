package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Name and context length limits, in Unicode code points after trimming
// leading and trailing Unicode whitespace (Build decision 5).
const (
	nameMinRunes    = 1
	nameMaxRunes    = 60
	contextMaxRunes = 4000
)

// FieldError is a validation failure naming the JSON field that caused it,
// per Build decision 4's 400 body shape ({"error": ..., "field": ...}). The
// service and handlers map a *FieldError to 400; every other error from this
// package is a read failure and maps to 500.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// AgentProfileReader resolves an agent profile by id, for coordinator
// create/PATCH validation and profileStatus
// (docs/specs/coordinator/system-design/coordinators.md#validation).
// Satisfied by the agent settings service; injected to avoid importing its
// controller/store tiers. Returns sql.ErrNoRows when no profile matches id,
// matching internal/agent/settings/store's contract.
type AgentProfileReader interface {
	GetAgentProfile(ctx context.Context, id string) (*settingsmodels.AgentProfile, error)
}

// ExecutorProfileReader resolves an executor profile by id. Satisfied by the
// task service. Returns repoerrors.ErrExecutorProfileNotFound when no profile
// matches id.
type ExecutorProfileReader interface {
	GetExecutorProfile(ctx context.Context, id string) (*taskmodels.ExecutorProfile, error)
}

// ProfileStatus is one of the values profileStatus computes for a
// coordinator's agent or executor profile
// (docs/specs/coordinator/system-design/coordinators.md#validation).
type ProfileStatus string

// Profile status values.
const (
	ProfileStatusOK          ProfileStatus = "ok"
	ProfileStatusMissing     ProfileStatus = "missing"
	ProfileStatusPassthrough ProfileStatus = "passthrough"
)

// Validator validates coordinator create/PATCH input and computes
// profileStatus, through injected agent and executor profile readers.
type Validator struct {
	agents    AgentProfileReader
	executors ExecutorProfileReader
}

// NewValidator builds a Validator over the given profile readers.
func NewValidator(agents AgentProfileReader, executors ExecutorProfileReader) *Validator {
	return &Validator{agents: agents, executors: executors}
}

// ValidateName trims raw of leading and trailing Unicode whitespace and
// checks its length is 1 to 60 Unicode code points, returning the trimmed
// value.
func ValidateName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(trimmed); n < nameMinRunes || n > nameMaxRunes {
		return "", &FieldError{
			Field:   "name",
			Message: fmt.Sprintf("name must be %d to %d characters", nameMinRunes, nameMaxRunes),
		}
	}
	return trimmed, nil
}

// ValidateContext trims raw of leading and trailing Unicode whitespace and
// checks its length is at most 4,000 Unicode code points, returning the
// trimmed value. An absent or all-whitespace context is valid and trims to
// "".
func ValidateContext(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if utf8.RuneCountInString(trimmed) > contextMaxRunes {
		return "", &FieldError{
			Field:   "context",
			Message: fmt.Sprintf("context must be at most %d characters", contextMaxRunes),
		}
	}
	return trimmed, nil
}

// ValidateAgentProfile returns a *FieldError naming "agent_profile_id" when
// agentProfileID does not resolve to an ok agent profile for workspaceID
// (missing, cross-workspace, or CLI-passthrough), or a plain error when the
// profile read itself fails.
func (v *Validator) ValidateAgentProfile(ctx context.Context, workspaceID, agentProfileID string) error {
	status, err := v.agentProfileStatus(ctx, workspaceID, agentProfileID)
	if err != nil {
		return err
	}
	switch status {
	case ProfileStatusMissing:
		return &FieldError{Field: "agent_profile_id", Message: "agent profile not found"}
	case ProfileStatusPassthrough:
		return &FieldError{
			Field:   "agent_profile_id",
			Message: "the coordinator needs Kandev's MCP tools, and this agent profile uses CLI passthrough",
		}
	}
	return nil
}

// ValidateExecutorProfile returns a *FieldError naming "executor_profile_id"
// when executorProfileID does not resolve to an existing executor profile, or
// a plain error when the profile read itself fails.
func (v *Validator) ValidateExecutorProfile(ctx context.Context, executorProfileID string) error {
	status, err := v.executorProfileStatus(ctx, executorProfileID)
	if err != nil {
		return err
	}
	if status == ProfileStatusMissing {
		return &FieldError{Field: "executor_profile_id", Message: "executor profile not found"}
	}
	return nil
}

// ProfileStatus computes the agent and executor profile statuses the
// coordinator GET, the conversation route and session start all report
// (docs/specs/coordinator/system-design/coordinators.md#validation). A read
// failure other than not-found returns it as a plain error and both statuses
// empty; it is never reported as ProfileStatusMissing.
func (v *Validator) ProfileStatus(ctx context.Context, workspaceID, agentProfileID, executorProfileID string) (agentStatus, executorStatus ProfileStatus, err error) {
	agentStatus, err = v.agentProfileStatus(ctx, workspaceID, agentProfileID)
	if err != nil {
		return "", "", err
	}
	executorStatus, err = v.executorProfileStatus(ctx, executorProfileID)
	if err != nil {
		return "", "", err
	}
	return agentStatus, executorStatus, nil
}

// agentProfileStatus resolves agentProfileID: missing when no row matches or
// the row's WorkspaceID is non-empty and differs from workspaceID (Build
// decision 6), passthrough when the row's CLIPassthrough is set, else ok. A
// read error other than sql.ErrNoRows is returned unwrapped.
func (v *Validator) agentProfileStatus(ctx context.Context, workspaceID, agentProfileID string) (ProfileStatus, error) {
	profile, err := v.agents.GetAgentProfile(ctx, agentProfileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProfileStatusMissing, nil
		}
		return "", fmt.Errorf("get agent profile: %w", err)
	}
	if profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID {
		return ProfileStatusMissing, nil
	}
	if profile.CLIPassthrough {
		return ProfileStatusPassthrough, nil
	}
	return ProfileStatusOK, nil
}

// executorProfileStatus resolves executorProfileID: missing when no row
// matches, else ok. A read error other than
// repoerrors.ErrExecutorProfileNotFound is returned unwrapped.
func (v *Validator) executorProfileStatus(ctx context.Context, executorProfileID string) (ProfileStatus, error) {
	_, err := v.executors.GetExecutorProfile(ctx, executorProfileID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
			return ProfileStatusMissing, nil
		}
		return "", fmt.Errorf("get executor profile: %w", err)
	}
	return ProfileStatusOK, nil
}
