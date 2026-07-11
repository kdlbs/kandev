package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeForegroundActivityProvider is a stand-in for the orchestrator's in-memory
// tracker so the serialization layer can be tested without importing it.
type fakeForegroundActivityProvider struct {
	value  v1.ForegroundActivity
	called []string
}

func (f *fakeForegroundActivityProvider) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	f.called = append(f.called, sessionID)
	return f.value
}

func TestEnrichForegroundActivity(t *testing.T) {
	tests := []struct {
		name     string
		state    models.TaskSessionState
		provider *fakeForegroundActivityProvider
		want     v1.ForegroundActivity
		// wantQueried asserts whether the provider was consulted — a non-RUNNING
		// session must never be queried so we never fabricate a substate.
		wantQueried bool
	}{
		{
			name:        "running background is surfaced",
			state:       models.TaskSessionStateRunning,
			provider:    &fakeForegroundActivityProvider{value: v1.ForegroundActivityBackground},
			want:        v1.ForegroundActivityBackground,
			wantQueried: true,
		},
		{
			name:        "running generating is surfaced",
			state:       models.TaskSessionStateRunning,
			provider:    &fakeForegroundActivityProvider{value: v1.ForegroundActivityGenerating},
			want:        v1.ForegroundActivityGenerating,
			wantQueried: true,
		},
		{
			name:        "non-running is left empty and never queried",
			state:       models.TaskSessionStateWaitingForInput,
			provider:    &fakeForegroundActivityProvider{value: v1.ForegroundActivityBackground},
			want:        "",
			wantQueried: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dto := &TaskSessionDTO{ID: "s1", State: tc.state}
			EnrichForegroundActivity(dto, tc.provider)
			if dto.ForegroundActivity != tc.want {
				t.Fatalf("full DTO: got %q, want %q", dto.ForegroundActivity, tc.want)
			}
			if queried := len(tc.provider.called) > 0; queried != tc.wantQueried {
				t.Fatalf("full DTO: provider queried=%v, want %v", queried, tc.wantQueried)
			}

			summary := &TaskSessionSummaryDTO{ID: "s1", State: tc.state}
			EnrichForegroundActivitySummary(summary, tc.provider)
			if summary.ForegroundActivity != tc.want {
				t.Fatalf("summary DTO: got %q, want %q", summary.ForegroundActivity, tc.want)
			}
		})
	}
}

func TestEnrichForegroundActivity_NilProviderIsNoOp(t *testing.T) {
	dto := &TaskSessionDTO{ID: "s1", State: models.TaskSessionStateRunning}
	EnrichForegroundActivity(dto, nil)
	if dto.ForegroundActivity != "" {
		t.Fatalf("nil provider must not set a substate, got %q", dto.ForegroundActivity)
	}

	summary := &TaskSessionSummaryDTO{ID: "s1", State: models.TaskSessionStateRunning}
	EnrichForegroundActivitySummary(summary, nil)
	if summary.ForegroundActivity != "" {
		t.Fatalf("nil provider must not set a substate, got %q", summary.ForegroundActivity)
	}
}
