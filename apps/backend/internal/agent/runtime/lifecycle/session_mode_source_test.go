package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type stubWorkspaceInfoProvider struct {
	info *WorkspaceInfo
	err  error
}

func (s *stubWorkspaceInfoProvider) GetWorkspaceInfoForSession(_ context.Context, _, _ string) (*WorkspaceInfo, error) {
	return s.info, s.err
}

func (s *stubWorkspaceInfoProvider) GetWorkspaceInfoForEnvironment(_ context.Context, _ string) (*WorkspaceInfo, error) {
	return s.info, s.err
}

func TestApplyExplicitSessionModeReportsUnavailableClient(t *testing.T) {
	execution := &AgentExecution{ID: "exec-mode", TaskID: "task-mode", SessionID: "session-mode"}
	manager := &SessionManager{logger: newTestLogger()}
	err := manager.applyExplicitSessionMode(context.Background(), execution, "acp-session", "plan")
	var failure *BootstrapFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T does not preserve typed bootstrap evidence: %v", err, err)
	}
	if failure.Code != models.AgentErrorCauseCodePermissionModeFailed || failure.Reason != models.AgentErrorCauseReasonClientUnavailable {
		t.Errorf("cause = (%q, %q), want (permission_mode_failed, client_unavailable)", failure.Code, failure.Reason)
	}
	if failure.RequestedMode != "plan" || failure.EffectiveMode != "" {
		t.Errorf("mode evidence = (%q, %q), want (plan, empty)", failure.RequestedMode, failure.EffectiveMode)
	}
	if failure.PromptNotSent == nil || !*failure.PromptNotSent {
		t.Error("prompt_not_sent evidence = false or unknown, want true")
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.4, .6
// A profile mode that loses to a persisted override used to be invisible: the
// session simply ran in a mode nobody could trace back to a source.
func TestEffectiveSessionModeReportsWinningSource(t *testing.T) {
	tests := []struct {
		name        string
		sessionMode string
		profileMode string
		wantMode    string
		wantSource  ModeSource
	}{
		{
			name:        "persisted override wins",
			sessionMode: "acceptEdits",
			profileMode: "bypassPermissions",
			wantMode:    "acceptEdits",
			wantSource:  ModeSourceSessionOverride,
		},
		{
			name:        "profile mode when nothing is persisted",
			profileMode: "bypassPermissions",
			wantMode:    "bypassPermissions",
			wantSource:  ModeSourceAgentProfile,
		},
		{
			name:       "no mode requested at all",
			wantMode:   "",
			wantSource: ModeSourceNone,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &Manager{
				logger: newTestLogger(),
				workspaceInfoProvider: &stubWorkspaceInfoProvider{
					info: &WorkspaceInfo{SessionMode: test.sessionMode},
				},
			}
			execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}

			mode, source := m.effectiveSessionModeWithSource(context.Background(), execution, test.profileMode)

			if mode != test.wantMode {
				t.Fatalf("mode = %q, want %q", mode, test.wantMode)
			}
			if source != test.wantSource {
				t.Fatalf("source = %q, want %q", source, test.wantSource)
			}
		})
	}
}
