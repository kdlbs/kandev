package lifecycle

import (
	"context"
	"testing"
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
