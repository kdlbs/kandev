package backendapp

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const tierUsageRateWindow = 10 * time.Minute

// tierUsageSnapshot holds the last provider-usage reading observed through the
// existing settings read surface. It never fetches: a profile that has not been
// observed stays unknown, and the engine ranks unknown below measured capacity.
type tierUsageSnapshot struct {
	mu        sync.RWMutex
	byProfile map[string]dynamic.CandidateUsage
}

func newTierUsageSnapshot() *tierUsageSnapshot {
	return &tierUsageSnapshot{byProfile: map[string]dynamic.CandidateUsage{}}
}

type profileUsageLister interface {
	ListProfileUsage(context.Context) ([]dto.AgentProfileUsageDTO, error)
	agentsettingscontroller.ProfileTurnAllowanceProvider
}

// tierUsageRecordingProvider records every completed read so tier selection can
// reuse the same account grouping and window provenance without another call.
// It forwards the turn-allowance surface so wrapping does not hide it from the
// settings controller.
type tierUsageRecordingProvider struct {
	next profileUsageLister
	sink *tierUsageSnapshot
}

var _ agentsettingscontroller.ProfileTurnAllowanceProvider = (*tierUsageRecordingProvider)(nil)

func (p *tierUsageRecordingProvider) GetProfileTurnAllowance(ctx context.Context, id string) (*dto.ProfileTurnAllowanceDTO, error) {
	return p.next.GetProfileTurnAllowance(ctx, id)
}

func (p *tierUsageRecordingProvider) PatchProfileTurnAllowance(ctx context.Context, id string, patch json.RawMessage) (*dto.ProfileTurnAllowanceDTO, error) {
	return p.next.PatchProfileTurnAllowance(ctx, id, patch)
}

func (p *tierUsageRecordingProvider) ListProfileUsage(ctx context.Context) ([]dto.AgentProfileUsageDTO, error) {
	profiles, err := p.next.ListProfileUsage(ctx)
	if err == nil {
		p.sink.Observe(profiles)
	}
	return profiles, err
}

func (s *tierUsageSnapshot) Observe(profiles []dto.AgentProfileUsageDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]dynamic.CandidateUsage, len(profiles))
	for _, profile := range profiles {
		usage, ok := candidateUsageFromDTO(profile)
		if !ok {
			continue
		}
		if prior, exists := s.byProfile[profile.ProfileID]; exists {
			usage = withTierConsumptionRate(prior, usage)
		}
		next[profile.ProfileID] = usage
	}
	s.byProfile = next
}

func (s *tierUsageSnapshot) Lookup(profileID string) dynamic.CandidateUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byProfile[profileID]
}

// candidateUsageFromDTO keeps the most constraining observed window. The
// account ID is P2's opaque provider:credential grouping; no credential path,
// fingerprint or secret reaches the engine.
func candidateUsageFromDTO(profile dto.AgentProfileUsageDTO) (dynamic.CandidateUsage, bool) {
	if profile.State != "ok" || profile.Stale || profile.FetchedAt == nil || profile.Provider == "" || profile.AccountID == "" {
		return dynamic.CandidateUsage{}, false
	}
	window, ok := constrainingWindow(profile.Windows)
	if !ok {
		return dynamic.CandidateUsage{}, false
	}
	return usageFromWindow(profile, window), true
}

func constrainingWindow(windows []dto.AgentProfileUsageWindowDTO) (dto.AgentProfileUsageWindowDTO, bool) {
	best := -1
	for index, window := range windows {
		if window.Label == "" {
			continue
		}
		if best < 0 || window.UtilizationPct > windows[best].UtilizationPct ||
			(window.UtilizationPct == windows[best].UtilizationPct && window.Label < windows[best].Label) {
			best = index
		}
	}
	if best < 0 {
		return dto.AgentProfileUsageWindowDTO{}, false
	}
	return windows[best], true
}

func usageFromWindow(profile dto.AgentProfileUsageDTO, window dto.AgentProfileUsageWindowDTO) dynamic.CandidateUsage {
	remaining := 100 - window.UtilizationPct
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	usage := dynamic.CandidateUsage{
		Bucket: dynamic.UsageBucket{
			Provider: profile.Provider, AccountRef: profile.AccountID, Window: window.Label,
		},
		FetchedAt:    *profile.FetchedAt,
		Known:        true,
		RemainingPct: remaining,
		Limited:      window.LimitReached || remaining <= 0,
	}
	if window.ResetAt != nil && window.ResetAt.After(*profile.FetchedAt) {
		usage.ResetAt = *window.ResetAt
		usage.ResetAtKnown = true
	}
	return usage
}

// withTierConsumptionRate derives a burn rate only from two comparable reads of
// the same provider/account/window with an unchanged proved reset. A reset,
// account change, non-increasing timestamp or unresolved window stays unknown.
func withTierConsumptionRate(prior, current dynamic.CandidateUsage) dynamic.CandidateUsage {
	if !prior.Known || !current.Known || prior.Bucket != current.Bucket {
		return current
	}
	interval := current.FetchedAt.Sub(prior.FetchedAt)
	if interval <= 0 || interval > tierUsageRateWindow {
		return current
	}
	if prior.RemainingPct < current.RemainingPct {
		return current
	}
	// A rate needs two proved, unchanged resets. A missing reset on either
	// sample cannot establish that the two readings cover the same window.
	if !prior.ResetAtKnown || !current.ResetAtKnown || !prior.ResetAt.Equal(current.ResetAt) {
		return current
	}
	current.ConsumptionRateKnown = true
	current.ConsumptionPctPerHour = (prior.RemainingPct - current.RemainingPct) / interval.Hours()
	return current
}
