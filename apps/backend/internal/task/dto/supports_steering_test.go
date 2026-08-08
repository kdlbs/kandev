package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// steerProvider implements both the activity and the steer-eligible provider
// interfaces so the serialization layer can be exercised without importing the
// orchestrator.
type steerProvider struct {
	activity     v1.ForegroundActivity
	steerable    bool
	steerQueried []string
}

func (p *steerProvider) ForegroundActivity(string) v1.ForegroundActivity { return p.activity }

func (p *steerProvider) SteerEligible(sessionID string, _ models.TaskSessionState) bool {
	p.steerQueried = append(p.steerQueried, sessionID)
	return p.steerable
}

func TestEnrichForegroundActivity_StampsSupportsSteering(t *testing.T) {
	for _, tc := range []struct {
		name      string
		steerable bool
	}{
		{name: "eligible", steerable: true},
		{name: "not eligible", steerable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &steerProvider{activity: v1.ForegroundActivityGenerating, steerable: tc.steerable}
			dto := &TaskSessionDTO{ID: "s1", State: models.TaskSessionStateRunning}
			EnrichForegroundActivity(dto, provider)
			if dto.SupportsSteering != tc.steerable {
				t.Fatalf("SupportsSteering = %t, want %t", dto.SupportsSteering, tc.steerable)
			}
			if len(provider.steerQueried) != 1 || provider.steerQueried[0] != "s1" {
				t.Fatalf("provider steer query = %v, want [s1]", provider.steerQueried)
			}
		})
	}
}

func TestEnrichForegroundActivitySummary_StampsSupportsSteering(t *testing.T) {
	provider := &steerProvider{activity: v1.ForegroundActivityGenerating, steerable: true}
	dto := &TaskSessionSummaryDTO{ID: "s2", State: models.TaskSessionStateRunning}
	EnrichForegroundActivitySummary(dto, provider)
	if !dto.SupportsSteering {
		t.Fatal("summary SupportsSteering = false, want true")
	}
}

// TestSupportsSteering_NotSetByFromTaskSession pins that the flag is a live
// serialization-boundary value, never derived from the persisted row. A restart
// with no connected provider must serialize false.
func TestSupportsSteering_NotSetByFromTaskSession(t *testing.T) {
	session := &models.TaskSession{ID: "s3", State: models.TaskSessionStateRunning}
	dto := FromTaskSession(session)
	if dto.SupportsSteering {
		t.Fatal("FromTaskSession set SupportsSteering; it must only be stamped by enrichment")
	}
}

// TestSupportsSteering_ProviderWithoutSteerInterface pins the conservative
// default: a provider that does not implement SteerEligibleProvider never
// advertises steering.
func TestSupportsSteering_ProviderWithoutSteerInterface(t *testing.T) {
	// fakeForegroundActivityProvider (from foreground_activity_test.go) has no
	// SteerEligible method.
	provider := &fakeForegroundActivityProvider{value: v1.ForegroundActivityGenerating}
	dto := &TaskSessionDTO{ID: "s4", State: models.TaskSessionStateRunning}
	EnrichForegroundActivity(dto, provider)
	if dto.SupportsSteering {
		t.Fatal("provider without SteerEligible advertised steering")
	}
}
