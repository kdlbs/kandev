package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"strings"
	"sync"
	"time"
)

type requestLimitObservation struct {
	profile, key, model string
	modelConflict       bool
	at                  time.Time
}
type requestLimitObservations struct {
	mu     sync.Mutex
	events []requestLimitObservation
	seen   map[string]time.Time
}

func (r *requestLimitObservations) count(ids []string, start, end time.Time) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	profiles := make(map[string]bool, len(ids))
	for _, id := range ids {
		profiles[id] = true
	}
	var count float64
	for _, e := range r.events {
		if profiles[e.profile] && !e.at.Before(start) && e.at.Before(end) {
			count++
		}
	}
	return count
}

func (r *requestLimitObservations) record(profile, key string, at time.Time) {
	r.recordForModel(profile, key, "", at)
}

func (r *requestLimitObservations) recordForModel(profile, key, model string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = make(map[string]time.Time)
	}
	cutoff := at.Add(-24 * time.Hour)
	for len(r.events) > 0 && r.events[0].at.Before(cutoff) {
		delete(r.seen, r.events[0].key)
		r.events = r.events[1:]
	}
	if _, exists := r.seen[key]; exists {
		r.updateModel(key, model)
		return
	}
	r.seen[key] = at
	r.events = append(r.events, requestLimitObservation{profile: profile, key: key, model: model, at: at})
}

func (r *requestLimitObservations) updateModel(key, model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	for index := len(r.events) - 1; index >= 0; index-- {
		event := &r.events[index]
		if event.key != key || event.modelConflict {
			continue
		}
		if event.model == "" {
			event.model = model
		} else if !strings.EqualFold(agentusage.ScopeModelID(event.model), agentusage.ScopeModelID(model)) {
			event.model, event.modelConflict = "", true
		}
		return
	}
}

// countModels reports whether all observed request models in the window are
// attributable; unknown observations are never assigned to a model scope.
func (r *requestLimitObservations) countModels(ids, models []string, start, end time.Time) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	profiles := make(map[string]bool, len(ids))
	for _, id := range ids {
		profiles[id] = true
	}
	var count float64
	for _, event := range r.events {
		if !profiles[event.profile] || event.at.Before(start) || !event.at.Before(end) {
			continue
		}
		if strings.TrimSpace(event.model) == "" || event.modelConflict {
			return 0, false
		}
		for _, model := range models {
			if strings.EqualFold(agentusage.ScopeModelID(event.model), agentusage.ScopeModelID(model)) {
				count++
				break
			}
		}
	}
	return count, true
}

// Subscribe estimates response boundaries on the existing agent usage stream.
// Assistant message identities are estimates, not HTTP request receipts.
func (s *providerTierLimitService) Subscribe(eventBus bus.EventBus) (bus.Subscription, error) {
	return eventBus.Subscribe(events.BuildAgentStreamWildcardSubject(), s.observeStreamRequests)
}

func (s *providerTierLimitService) observeStreamRequests(_ context.Context, event *bus.Event) error {
	var payload runtimeapi.AgentStreamEventPayload
	encoded, err := json.Marshal(event.Data)
	if err != nil {
		return nil
	}
	if json.Unmarshal(encoded, &payload) != nil || payload.Data == nil {
		return nil
	}
	profile := payload.ExecutionProfileID
	if profile == "" {
		profile = payload.AgentProfileID
	}
	if profile == "" {
		return nil
	}
	identity := streamRequestIdentity(payload)
	if identity == "" {
		return nil
	}
	at := event.Timestamp
	if at.IsZero() {
		at = time.Now()
	}
	key := fmt.Sprintf("%s:%s:%s:%d:%s", profile, payload.ExecutionID, payload.AttemptID, payload.Data.PromptGeneration, identity)
	model := ""
	if payload.Data.UsageObservation != nil {
		model = payload.Data.UsageObservation.Model
	}
	s.requests.recordForModel(profile, key, model, at)
	return nil
}

func streamRequestIdentity(payload runtimeapi.AgentStreamEventPayload) string {
	data := payload.Data
	if data.UsageObservation != nil && data.UsageObservation.ProviderResponseID != "" {
		return data.UsageObservation.ProviderResponseID
	}
	if payload.AgentType == codexAppServerAgentID || data.Type != "message" {
		return ""
	}
	if data.ProtocolMessageID != "" {
		return data.ProtocolMessageID
	}
	return data.MessageID
}
