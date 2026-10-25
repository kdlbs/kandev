package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// fakeAgentProfileReader is a test double for AgentProfileReader.
type fakeAgentProfileReader struct {
	profiles map[string]*settingsmodels.AgentProfile
	err      error // when set, returned for every id (simulates a read failure)
}

func (f *fakeAgentProfileReader) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	profile, ok := f.profiles[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return profile, nil
}

// fakeExecutorProfileReader is a test double for ExecutorProfileReader.
type fakeExecutorProfileReader struct {
	profiles map[string]*taskmodels.ExecutorProfile
	err      error
}

func (f *fakeExecutorProfileReader) GetExecutorProfile(_ context.Context, id string) (*taskmodels.ExecutorProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	profile, ok := f.profiles[id]
	if !ok {
		return nil, repoerrors.ErrExecutorProfileNotFound
	}
	return profile, nil
}

func newValidatorForTest(agents map[string]*settingsmodels.AgentProfile, executors map[string]*taskmodels.ExecutorProfile) *Validator {
	return NewValidator(
		&fakeAgentProfileReader{profiles: agents},
		&fakeExecutorProfileReader{profiles: executors},
	)
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "trims and keeps a normal name", raw: "  Release Coordinator  ", want: "Release Coordinator"},
		{name: "single character is valid", raw: "x", want: "x"},
		{name: "exactly 60 runes is valid", raw: strings.Repeat("a", 60), want: strings.Repeat("a", 60)},
		{name: "empty after trim is invalid", raw: "   ", wantErr: true},
		{name: "61 runes is invalid", raw: strings.Repeat("a", 61), wantErr: true},
		{name: "60 multi-byte runes is valid", raw: strings.Repeat("é", 60), want: strings.Repeat("é", 60)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateName(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateName(%q) error = nil, want error", tc.raw)
				}
				var fieldErr *FieldError
				if !errors.As(err, &fieldErr) {
					t.Fatalf("ValidateName(%q) error type = %T, want *FieldError", tc.raw, err)
				}
				if fieldErr.Field != "name" {
					t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "name")
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateName(%q) unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ValidateName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestValidateContext(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "trims and keeps context", raw: "  standing context  ", want: "standing context"},
		{name: "empty is valid", raw: "   ", want: ""},
		{name: "exactly 4000 runes is valid", raw: strings.Repeat("a", 4000), want: strings.Repeat("a", 4000)},
		{name: "4001 runes is invalid", raw: strings.Repeat("a", 4001), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateContext(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateContext() error = nil, want error")
				}
				var fieldErr *FieldError
				if !errors.As(err, &fieldErr) {
					t.Fatalf("ValidateContext() error type = %T, want *FieldError", err)
				}
				if fieldErr.Field != "context" {
					t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "context")
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateContext() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ValidateContext() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidatorValidateAgentProfile(t *testing.T) {
	const workspaceID = "ws-1"

	t.Run("ok profile in the same workspace passes", func(t *testing.T) {
		v := newValidatorForTest(map[string]*settingsmodels.AgentProfile{
			"ap-1": {ID: "ap-1", WorkspaceID: workspaceID},
		}, nil)
		if err := v.ValidateAgentProfile(context.Background(), workspaceID, "ap-1"); err != nil {
			t.Fatalf("ValidateAgentProfile() unexpected error: %v", err)
		}
	})

	t.Run("global profile (empty workspace_id) passes", func(t *testing.T) {
		v := newValidatorForTest(map[string]*settingsmodels.AgentProfile{
			"ap-1": {ID: "ap-1", WorkspaceID: ""},
		}, nil)
		if err := v.ValidateAgentProfile(context.Background(), workspaceID, "ap-1"); err != nil {
			t.Fatalf("ValidateAgentProfile() unexpected error: %v", err)
		}
	})

	t.Run("missing profile is a 400 naming agent_profile_id", func(t *testing.T) {
		v := newValidatorForTest(nil, nil)
		err := v.ValidateAgentProfile(context.Background(), workspaceID, "does-not-exist")
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("ValidateAgentProfile() error type = %T, want *FieldError", err)
		}
		if fieldErr.Field != "agent_profile_id" {
			t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "agent_profile_id")
		}
	})

	t.Run("profile belonging to a different workspace is treated as missing", func(t *testing.T) {
		v := newValidatorForTest(map[string]*settingsmodels.AgentProfile{
			"ap-1": {ID: "ap-1", WorkspaceID: "other-workspace"},
		}, nil)
		err := v.ValidateAgentProfile(context.Background(), workspaceID, "ap-1")
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("ValidateAgentProfile() error type = %T, want *FieldError", err)
		}
		if fieldErr.Field != "agent_profile_id" {
			t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "agent_profile_id")
		}
	})

	t.Run("passthrough profile is a 400 naming agent_profile_id", func(t *testing.T) {
		v := newValidatorForTest(map[string]*settingsmodels.AgentProfile{
			"ap-1": {ID: "ap-1", WorkspaceID: workspaceID, CLIPassthrough: true},
		}, nil)
		err := v.ValidateAgentProfile(context.Background(), workspaceID, "ap-1")
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("ValidateAgentProfile() error type = %T, want *FieldError", err)
		}
		if fieldErr.Field != "agent_profile_id" {
			t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "agent_profile_id")
		}
	})

	t.Run("a read failure other than not-found is a plain error, never a FieldError", func(t *testing.T) {
		v := NewValidator(&fakeAgentProfileReader{err: errors.New("boom")}, &fakeExecutorProfileReader{})
		err := v.ValidateAgentProfile(context.Background(), workspaceID, "ap-1")
		if err == nil {
			t.Fatal("ValidateAgentProfile() error = nil, want error")
		}
		var fieldErr *FieldError
		if errors.As(err, &fieldErr) {
			t.Fatalf("ValidateAgentProfile() error = %v, want a plain (non-field) error", err)
		}
	})
}

func TestValidatorValidateExecutorProfile(t *testing.T) {
	t.Run("existing profile passes", func(t *testing.T) {
		v := newValidatorForTest(nil, map[string]*taskmodels.ExecutorProfile{
			"ep-1": {ID: "ep-1"},
		})
		if err := v.ValidateExecutorProfile(context.Background(), "ep-1"); err != nil {
			t.Fatalf("ValidateExecutorProfile() unexpected error: %v", err)
		}
	})

	t.Run("missing profile is a 400 naming executor_profile_id", func(t *testing.T) {
		v := newValidatorForTest(nil, nil)
		err := v.ValidateExecutorProfile(context.Background(), "does-not-exist")
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("ValidateExecutorProfile() error type = %T, want *FieldError", err)
		}
		if fieldErr.Field != "executor_profile_id" {
			t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, "executor_profile_id")
		}
	})

	t.Run("a read failure other than not-found is a plain error, never a FieldError", func(t *testing.T) {
		v := NewValidator(&fakeAgentProfileReader{}, &fakeExecutorProfileReader{err: errors.New("boom")})
		err := v.ValidateExecutorProfile(context.Background(), "ep-1")
		if err == nil {
			t.Fatal("ValidateExecutorProfile() error = nil, want error")
		}
		var fieldErr *FieldError
		if errors.As(err, &fieldErr) {
			t.Fatalf("ValidateExecutorProfile() error = %v, want a plain (non-field) error", err)
		}
	})
}

func TestValidatorProfileStatus(t *testing.T) {
	const workspaceID = "ws-1"

	t.Run("ok/ok for existing, same-workspace, non-passthrough profiles", func(t *testing.T) {
		v := newValidatorForTest(
			map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}},
			map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}},
		)
		agentStatus, executorStatus, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err != nil {
			t.Fatalf("ProfileStatus() unexpected error: %v", err)
		}
		if agentStatus != ProfileStatusOK || executorStatus != ProfileStatusOK {
			t.Errorf("ProfileStatus() = (%q, %q), want (ok, ok)", agentStatus, executorStatus)
		}
	})

	t.Run("missing/missing for absent profiles", func(t *testing.T) {
		v := newValidatorForTest(nil, nil)
		agentStatus, executorStatus, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err != nil {
			t.Fatalf("ProfileStatus() unexpected error: %v", err)
		}
		if agentStatus != ProfileStatusMissing || executorStatus != ProfileStatusMissing {
			t.Errorf("ProfileStatus() = (%q, %q), want (missing, missing)", agentStatus, executorStatus)
		}
	})

	t.Run("passthrough for a passthrough agent profile", func(t *testing.T) {
		v := newValidatorForTest(
			map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID, CLIPassthrough: true}},
			map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}},
		)
		agentStatus, _, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err != nil {
			t.Fatalf("ProfileStatus() unexpected error: %v", err)
		}
		if agentStatus != ProfileStatusPassthrough {
			t.Errorf("agentStatus = %q, want %q", agentStatus, ProfileStatusPassthrough)
		}
	})

	t.Run("missing for a cross-workspace agent profile", func(t *testing.T) {
		v := newValidatorForTest(
			map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: "other-workspace"}},
			map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}},
		)
		agentStatus, _, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err != nil {
			t.Fatalf("ProfileStatus() unexpected error: %v", err)
		}
		if agentStatus != ProfileStatusMissing {
			t.Errorf("agentStatus = %q, want %q", agentStatus, ProfileStatusMissing)
		}
	})

	t.Run("a non-not-found agent profile read failure returns an error and never missing", func(t *testing.T) {
		v := NewValidator(&fakeAgentProfileReader{err: errors.New("boom")}, &fakeExecutorProfileReader{
			profiles: map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}},
		})
		agentStatus, executorStatus, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err == nil {
			t.Fatal("ProfileStatus() error = nil, want error")
		}
		if agentStatus != "" || executorStatus != "" {
			t.Errorf("ProfileStatus() on error = (%q, %q), want empty statuses", agentStatus, executorStatus)
		}
	})

	t.Run("a non-not-found executor profile read failure returns an error and never missing", func(t *testing.T) {
		v := NewValidator(
			&fakeAgentProfileReader{profiles: map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}},
			&fakeExecutorProfileReader{err: errors.New("boom")},
		)
		agentStatus, executorStatus, err := v.ProfileStatus(context.Background(), workspaceID, "ap-1", "ep-1")
		if err == nil {
			t.Fatal("ProfileStatus() error = nil, want error")
		}
		if agentStatus != "" || executorStatus != "" {
			t.Errorf("ProfileStatus() on error = (%q, %q), want empty statuses", agentStatus, executorStatus)
		}
	})
}
