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
		// wantQueried asserts whether the provider was consulted.
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
			name:        "non-running detached background is surfaced",
			state:       models.TaskSessionStateWaitingForInput,
			provider:    &fakeForegroundActivityProvider{value: v1.ForegroundActivityBackground},
			want:        v1.ForegroundActivityBackground,
			wantQueried: true,
		},
		{
			name:        "non-running generating fallback is omitted",
			state:       models.TaskSessionStateWaitingForInput,
			provider:    &fakeForegroundActivityProvider{value: v1.ForegroundActivityGenerating},
			want:        "",
			wantQueried: true,
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

// mapForegroundActivityProvider resolves a per-session activity so multi-session
// aggregation can be exercised with distinct values per session.
type mapForegroundActivityProvider struct {
	byID   map[string]v1.ForegroundActivity
	called []string
}

func (m *mapForegroundActivityProvider) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	m.called = append(m.called, sessionID)
	return m.byID[sessionID]
}

func TestEnrichTaskForegroundActivity(t *testing.T) {
	running := models.TaskSessionStateRunning
	done := models.TaskSessionStateCompleted
	waiting := models.TaskSessionStateWaitingForInput

	sess := func(id string, state models.TaskSessionState) *models.TaskSession {
		return &models.TaskSession{ID: id, State: state}
	}

	tests := []struct {
		name     string
		sessions []*models.TaskSession
		byID     map[string]v1.ForegroundActivity
		want     v1.ForegroundActivity
		// wantQueried lists the session IDs consulted for detached background
		// activity, including settled sessions.
		wantQueried []string
	}{
		{
			name:        "any generating wins",
			sessions:    []*models.TaskSession{sess("a", running), sess("b", running)},
			byID:        map[string]v1.ForegroundActivity{"a": v1.ForegroundActivityBackground, "b": v1.ForegroundActivityGenerating},
			want:        v1.ForegroundActivityGenerating,
			wantQueried: []string{"a", "b"},
		},
		{
			name:        "none generating but one background",
			sessions:    []*models.TaskSession{sess("a", running), sess("b", running)},
			byID:        map[string]v1.ForegroundActivity{"a": v1.ForegroundActivityBackground, "b": v1.ForegroundActivityBackground},
			want:        v1.ForegroundActivityBackground,
			wantQueried: []string{"a", "b"},
		},
		{
			name:        "finished primary does not mask a still-working secondary",
			sessions:    []*models.TaskSession{sess("primary", done), sess("secondary", running)},
			byID:        map[string]v1.ForegroundActivity{"secondary": v1.ForegroundActivityBackground},
			want:        v1.ForegroundActivityBackground,
			wantQueried: []string{"primary", "secondary"},
		},
		{
			name:        "no running session falls through to empty",
			sessions:    []*models.TaskSession{sess("a", done), sess("b", waiting)},
			byID:        map[string]v1.ForegroundActivity{},
			want:        "",
			wantQueried: []string{"a", "b"},
		},
		{
			name:        "nil sessions are skipped",
			sessions:    []*models.TaskSession{nil, sess("a", running)},
			byID:        map[string]v1.ForegroundActivity{"a": v1.ForegroundActivityGenerating},
			want:        v1.ForegroundActivityGenerating,
			wantQueried: []string{"a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &mapForegroundActivityProvider{byID: tc.byID}
			dto := &TaskDTO{ID: "t1"}
			EnrichTaskForegroundActivity(dto, tc.sessions, provider)
			if dto.ForegroundActivity != tc.want {
				t.Fatalf("aggregate: got %q, want %q", dto.ForegroundActivity, tc.want)
			}
			if len(provider.called) != len(tc.wantQueried) {
				t.Fatalf("queried %v, want %v", provider.called, tc.wantQueried)
			}
			for i, id := range tc.wantQueried {
				if provider.called[i] != id {
					t.Fatalf("queried[%d]=%q, want %q (all: %v)", i, provider.called[i], id, provider.called)
				}
			}
		})
	}
}

func TestEnrichTaskForegroundActivity_NilProviderIsNoOp(t *testing.T) {
	dto := &TaskDTO{ID: "t1"}
	EnrichTaskForegroundActivity(dto, []*models.TaskSession{{ID: "a", State: models.TaskSessionStateRunning}}, nil)
	if dto.ForegroundActivity != "" {
		t.Fatalf("nil provider must not set an aggregate, got %q", dto.ForegroundActivity)
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
