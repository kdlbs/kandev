package backendapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type tierStoreCall struct {
	ids        []string
	start, end time.Time
}

// fakeTierStore serves fixed counters. It does not implement
// modelWindowUsageReader, which models a store without model attribution.
type fakeTierStore struct {
	owner       agentusage.AccountTierConfig
	active      int64
	events      int64
	activeCalls []string
	calls       []tierStoreCall
}

func (s *fakeTierStore) LoadAccountTierConfig(context.Context, string) (agentusage.AccountTierConfig, agentusage.AccountTierConfig, error) {
	return s.owner, agentusage.AccountTierConfig{}, nil
}
func (s *fakeTierStore) SaveAccountTierOverrides(context.Context, string, agentusage.AccountTierConfig) error {
	return nil
}
func (s *fakeTierStore) SaveAccountTierLearned(context.Context, string, agentusage.AccountTierConfig) error {
	return nil
}
func (s *fakeTierStore) CountLimitActiveTurns(_ context.Context, _ []string, exclude string) (int64, error) {
	s.activeCalls = append(s.activeCalls, exclude)
	return s.active, nil
}
func (s *fakeTierStore) GetManualWindowUsageForProfiles(_ context.Context, ids []string, start, end time.Time) (sqliterepo.ManualWindowUsage, error) {
	s.calls = append(s.calls, tierStoreCall{ids: ids, start: start, end: end})
	return sqliterepo.ManualWindowUsage{EventCount: s.events}, nil
}
func (s *fakeTierStore) InsertUsageLimitObservation(context.Context, sqliterepo.UsageLimitObservation) error {
	return nil
}
func (s *fakeTierStore) ListUsageLimitObservationsSince(context.Context, time.Time) ([]sqliterepo.UsageLimitObservation, error) {
	return nil, nil
}

type fakeModelTierStore struct {
	fakeTierStore
	groups []sqliterepo.ManualModelWindowUsage
}

func (s *fakeModelTierStore) GetManualModelWindowUsageForProfiles(context.Context, []string, time.Time, time.Time) ([]sqliterepo.ManualModelWindowUsage, error) {
	return s.groups, nil
}

// tierSettings serves one agent and its profiles to the usage adapter.
type tierSettings struct {
	settingsstore.Repository
	profiles []*settingsmodels.AgentProfile
}

func (s *tierSettings) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	for _, p := range s.profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, errors.New("not found")
}
func (s *tierSettings) GetAgent(context.Context, string) (*settingsmodels.Agent, error) {
	return &settingsmodels.Agent{ID: "agent-1", Name: "custom-agent"}, nil
}
func (s *tierSettings) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	return []*settingsmodels.Agent{{ID: "agent-1", Name: "custom-agent"}}, nil
}
func (s *tierSettings) ListAgentProfiles(context.Context, string) ([]*settingsmodels.AgentProfile, error) {
	return s.profiles, nil
}

func tierTestNow() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }

func tierF(v float64) *float64 { return &v }

// newTierTestService wires a service whose profile "p1" is a paid-tier profile
// of its own account, so owner allowances come from the fake store.
func newTierTestService(t *testing.T, store tierLimitStore, owner agentusage.TierLimits) *providerTierLimitService {
	t.Helper()
	settings := &tierSettings{profiles: []*settingsmodels.AgentProfile{{ID: "p1", AgentID: "agent-1", Model: "acme/model-a"}}}
	adapter := newUsageProviderAdapter(settings, agentregistry.NewRegistry(newTestLogger()))
	svc := &providerTierLimitService{store: store, usage: adapter}
	adapter.tierLimits = svc
	switch s := store.(type) {
	case *fakeTierStore:
		s.owner = agentusage.AccountTierConfig{Tiers: map[agentusage.BillingTier]agentusage.TierLimits{agentusage.TierPaid: owner}}
	case *fakeModelTierStore:
		s.owner = agentusage.AccountTierConfig{Tiers: map[agentusage.BillingTier]agentusage.TierLimits{agentusage.TierPaid: owner}}
	}
	return svc
}

func TestCheckCandidateAdmitsBelowAndDeniesAtMinuteLimit(t *testing.T) {
	now := tierTestNow()
	cases := []struct {
		name   string
		events int64
		deny   bool
	}{
		{"just under", 2, false},
		{"at limit", 3, true},
		{"over limit", 4, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeTierStore{events: tc.events}
			svc := newTierTestService(t, store, agentusage.TierLimits{RequestsPerMinute: tierF(3)})
			got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
			if err != nil {
				t.Fatalf("CheckCandidate: %v", err)
			}
			if (got.Reason != "") != tc.deny {
				t.Fatalf("events=%d admission=%+v, deny=%v", tc.events, got, tc.deny)
			}
			if tc.deny && (got.Reason != "limit_minute" || !got.RetryAt.Equal(now.Add(time.Minute))) {
				t.Fatalf("admission = %+v", got)
			}
		})
	}
}

func TestCheckCandidateConcurrencyExcludesTheCandidateExecution(t *testing.T) {
	now := tierTestNow()
	store := &fakeTierStore{active: 2}
	svc := newTierTestService(t, store, agentusage.TierLimits{MaxConcurrentTurns: tierF(2)})
	got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "exec-7", now)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Reason != "limit_concurrency" {
		t.Fatalf("admission = %+v", got)
	}
	if len(store.activeCalls) != 1 || store.activeCalls[0] != "exec-7" {
		t.Fatalf("exclude passed to the store = %v", store.activeCalls)
	}
	store.active = 1
	if got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "exec-7", now); err != nil || got.Reason != "" {
		t.Fatalf("one active turn of 2: %+v %v", got, err)
	}
}

func TestCheckCandidateDailyLimitUsesResetWindow(t *testing.T) {
	now := tierTestNow()
	store := &fakeTierStore{events: 100}
	svc := newTierTestService(t, store, agentusage.TierLimits{RequestsPerDay: tierF(100), DailyReset: "06:00 UTC"})
	got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Reason != "limit_daily" || !got.RetryAt.Equal(time.Date(2026, 3, 11, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("admission = %+v", got)
	}
	// Day usage is read from the last 06:00 UTC reset up to now.
	wantStart := time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC)
	found := false
	for _, call := range store.calls {
		if call.start.Equal(wantStart) && call.end.Equal(now) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no day-window read from %v to %v: %+v", wantStart, now, store.calls)
	}

	store.events = 99
	if got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now); err != nil || got.Reason != "" {
		t.Fatalf("99 of 100: %+v %v", got, err)
	}
}

func TestCheckCandidateShortWindowReadsConfiguredSeconds(t *testing.T) {
	now := tierTestNow()
	store := &fakeTierStore{events: 5}
	svc := newTierTestService(t, store, agentusage.TierLimits{RequestsPerWindow: tierF(5), RequestWindowSeconds: 90})
	got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Reason != "limit_minute" || !got.RetryAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("admission = %+v", got)
	}
	found := false
	for _, call := range store.calls {
		if call.start.Equal(now.Add(-90*time.Second)) && call.end.Equal(now) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no 90-second window read: %+v", store.calls)
	}
}

func modelUsage(profile, model string, events int64) sqliterepo.ManualModelWindowUsage {
	return sqliterepo.ManualModelWindowUsage{ProfileID: profile, Model: model, Usage: sqliterepo.ManualWindowUsage{EventCount: events}}
}

func TestCheckCandidateAppliesModelAndGroupScopeCounts(t *testing.T) {
	now := tierTestNow()
	model := agentusage.RequestScope{ID: agentusage.ModelScopeID("model-a"), Kind: agentusage.ScopeKindModel, Models: []string{"model-a"}, RequestsPerDay: tierF(3)}
	group := agentusage.RequestScope{ID: "group:model-a+model-c", Kind: agentusage.ScopeKindGroup, Models: []string{"model-a", "model-c"}, RequestsPerDay: tierF(5)}
	other := agentusage.RequestScope{ID: agentusage.ModelScopeID("other"), Kind: agentusage.ScopeKindModel, Models: []string{"other"}, RequestsPerDay: tierF(1)}
	// Scope usage follows the recorded model of each request: p2 ran model-a
	// and model-c through overrides, and p1's own model-b requests never count.
	cases := []struct {
		name   string
		scopes []agentusage.RequestScope
		groups []sqliterepo.ManualModelWindowUsage
		deny   bool
	}{
		{"model scope just under", []agentusage.RequestScope{model}, []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 2), modelUsage("p1", "acme/model-b", 50)}, false},
		{"model scope at limit", []agentusage.RequestScope{model}, []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 2), modelUsage("p2", "acme/model-a", 1)}, true},
		{"group scope just under", []agentusage.RequestScope{group}, []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 2), modelUsage("p2", "acme/model-c", 2), modelUsage("p1", "acme/model-b", 50)}, false},
		{"group scope at limit", []agentusage.RequestScope{group}, []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 2), modelUsage("p2", "acme/model-c", 3)}, true},
		{"scope for another model never applies", []agentusage.RequestScope{other}, []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/other", 50)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTierTestService(t, &fakeModelTierStore{fakeTierStore: fakeTierStore{events: 50}, groups: tc.groups}, agentusage.TierLimits{Scopes: tc.scopes})
			got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
			if err != nil {
				t.Fatalf("CheckCandidate: %v", err)
			}
			if (got.Reason != "") != tc.deny {
				t.Fatalf("admission=%+v, deny=%v", got, tc.deny)
			}
			if tc.deny && (got.Reason != "limit_daily" || !got.RetryAt.Equal(time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC))) {
				t.Fatalf("admission = %+v", got)
			}
		})
	}
}

func TestCheckCandidateScopedBudgetCountsRecordedModels(t *testing.T) {
	now := tierTestNow()
	budget := agentusage.LimitBudget{Window: agentusage.WindowPeriodDay, Unit: "requests", Value: tierF(3), Reset: "rolling", Scope: string(agentusage.WindowScopePaidModels)}
	cases := []struct {
		name   string
		groups []sqliterepo.ManualModelWindowUsage
		deny   bool
	}{
		{"just under", []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 2)}, false},
		{"at limit", []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "acme/model-a", 3)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeModelTierStore{fakeTierStore: fakeTierStore{events: 99}, groups: tc.groups}
			svc := newTierTestService(t, store, agentusage.TierLimits{Windows: []agentusage.LimitBudget{budget}})
			got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
			if err != nil {
				t.Fatalf("CheckCandidate: %v", err)
			}
			if (got.Reason != "") != tc.deny {
				t.Fatalf("admission=%+v, deny=%v", got, tc.deny)
			}
		})
	}
}

func TestCheckCandidateKeepsAccountWideEvidenceAfterAttributionFailure(t *testing.T) {
	// A recorded request without a model leaves the per-model minute count
	// unknown, but the account-wide daily allowance is still read and applied.
	now := tierTestNow()
	store := &fakeModelTierStore{fakeTierStore: fakeTierStore{events: 10}, groups: []sqliterepo.ManualModelWindowUsage{modelUsage("p1", "", 1)}}
	svc := newTierTestService(t, store, agentusage.TierLimits{RequestsPerModel: true, RequestsPerMinute: tierF(1), RequestsPerDay: tierF(10)})
	got, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now)
	if err != nil || got.Reason != "limit_daily" {
		t.Fatalf("admission = %+v err %v, want limit_daily", got, err)
	}
	store.events = 9
	if _, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1"}, "", now); !errors.Is(err, errModelAttributionUnavailable) {
		t.Fatalf("unproven per-model minute must surface the attribution failure: %v", err)
	}
}

func TestCheckCandidateRejectsUnknownExecutionSourceAndUnmatchedSelection(t *testing.T) {
	now := tierTestNow()
	svc := newTierTestService(t, &fakeTierStore{}, agentusage.TierLimits{RequestsPerMinute: tierF(1)})

	_, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1", RequireExecutionUsage: true}, "", now)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("required execution usage without a policy: %v", err)
	}

	// A selection that no provider reading backs cannot prove its account, so
	// the candidate fails closed instead of being admitted on another account.
	policy := &agentusage.ExecutionUsagePolicy{ProfileID: "p1", ExpectedProvider: "acme", ExpectedAccountRef: "acct", ExpectedModel: "acme/model-a"}
	_, err = svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "p1", UsagePolicy: policy, RequireExecutionUsage: true}, "", now)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unmatched selection: %v", err)
	}
}

func TestCheckCandidateFailsForUnknownProfile(t *testing.T) {
	svc := newTierTestService(t, &fakeTierStore{}, agentusage.TierLimits{})
	if _, err := svc.CheckCandidate(context.Background(), dynamicruntime.Candidate{ID: "missing"}, "", tierTestNow()); err == nil {
		t.Fatal("unknown profile admitted")
	}
}

func tierContextFor(model string, perModel, override bool) tierLimitContext {
	return tierLimitContext{
		modelOverride: override,
		profile:       &settingsmodels.AgentProfile{ID: "p1", Model: model},
		effective:     agentusage.TierLimits{RequestsPerModel: perModel},
		ids:           []string{"p1", "p2"},
	}
}

func TestReadMinuteRequestCountAccountWideUsesLargerOfRecordedAndLive(t *testing.T) {
	now := tierTestNow()
	start, end := now.Add(-time.Minute), now
	ctx := context.Background()
	c := tierContextFor("acme/model-a", false, false)

	store := &fakeTierStore{events: 3}
	svc := &providerTierLimitService{store: store}
	for i, at := range []time.Time{start, now.Add(-30 * time.Second), end, start.Add(-time.Second)} {
		svc.requests.recordForModel("p1", string(rune('a'+i)), "acme/model-a", at)
	}
	// Live events inside [start, end): the one at start and the 30s one. The
	// event at end and the one before start are outside the window.
	got, err := svc.readMinuteRequestCount(ctx, c, start, end)
	if err != nil || got != 3 {
		t.Fatalf("recorded 3 > live 2: got %v err %v", got, err)
	}
	store.events = 1
	got, err = svc.readMinuteRequestCount(ctx, c, start, end)
	if err != nil || got != 2 {
		t.Fatalf("live 2 > recorded 1: got %v err %v", got, err)
	}
	if len(store.calls) != 2 || store.calls[0].start != start || store.calls[0].end != end {
		t.Fatalf("store window = %+v", store.calls)
	}
}

func TestReadMinuteRequestCountPerModelOverrideCountsOnlyThatModel(t *testing.T) {
	now := tierTestNow()
	start, end := now.Add(-time.Minute), now
	ctx := context.Background()
	c := tierContextFor("acme/model-a", true, true)

	store := &fakeModelTierStore{groups: []sqliterepo.ManualModelWindowUsage{
		{ProfileID: "p1", Model: "acme/model-a", Usage: sqliterepo.ManualWindowUsage{EventCount: 2}},
		{ProfileID: "p2", Model: "MODEL-A", Usage: sqliterepo.ManualWindowUsage{EventCount: 1}},
		{ProfileID: "p1", Model: "acme/model-b", Usage: sqliterepo.ManualWindowUsage{EventCount: 40}},
		{ProfileID: "p1", Model: "", Usage: sqliterepo.ManualWindowUsage{EventCount: 0}},
	}}
	svc := &providerTierLimitService{store: store}
	got, err := svc.readMinuteRequestCount(ctx, c, start, end)
	if err != nil || got != 3 {
		t.Fatalf("model-a recorded across profiles and prefix forms: got %v err %v", got, err)
	}

	// A larger live count for the same model wins; other models are ignored.
	for i, model := range []string{"acme/model-a", "acme/model-a", "acme/model-a", "acme/model-a", "acme/model-b"} {
		svc.requests.recordForModel("p1", string(rune('a'+i)), model, now.Add(-10*time.Second))
	}
	got, err = svc.readMinuteRequestCount(ctx, c, start, end)
	if err != nil || got != 4 {
		t.Fatalf("live 4 model-a events: got %v err %v", got, err)
	}
}

func TestReadMinuteRequestCountPerModelOverrideRequiresPerModelLimit(t *testing.T) {
	// Current behavior: without RequestsPerModel an overridden model still
	// uses the account-wide request count of the passed profile ids.
	now := tierTestNow()
	c := tierContextFor("acme/model-a", false, true)
	store := &fakeModelTierStore{
		fakeTierStore: fakeTierStore{events: 9},
		groups:        []sqliterepo.ManualModelWindowUsage{{Model: "acme/model-a", Usage: sqliterepo.ManualWindowUsage{EventCount: 1}}},
	}
	svc := &providerTierLimitService{store: store}
	got, err := svc.readMinuteRequestCount(context.Background(), c, now.Add(-time.Minute), now)
	if err != nil || got != 9 {
		t.Fatalf("account-wide count = %v err %v, want 9", got, err)
	}
}

func TestReadMinuteRequestCountPerModelCountsRecordedModelsWithoutOverride(t *testing.T) {
	// p1's configured model is model-a. p2 ran model-a through an override and
	// p1 also ran model-b; only the model-a requests count for p1's model.
	now := tierTestNow()
	c := tierContextFor("acme/model-a", true, false)
	store := &fakeModelTierStore{
		fakeTierStore: fakeTierStore{events: 41},
		groups: []sqliterepo.ManualModelWindowUsage{
			{ProfileID: "p1", Model: "acme/model-a", Usage: sqliterepo.ManualWindowUsage{EventCount: 2}},
			{ProfileID: "p2", Model: "acme/model-a", Usage: sqliterepo.ManualWindowUsage{EventCount: 1}},
			{ProfileID: "p1", Model: "acme/model-b", Usage: sqliterepo.ManualWindowUsage{EventCount: 38}},
		},
	}
	svc := &providerTierLimitService{store: store}
	got, err := svc.readMinuteRequestCount(context.Background(), c, now.Add(-time.Minute), now)
	if err != nil || got != 3 {
		t.Fatalf("recorded model-a requests = %v err %v, want 3", got, err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("a per-model count must not read the account-wide total: %+v", store.calls)
	}
}

func TestReadMinuteRequestCountModelAttributionFailures(t *testing.T) {
	now := tierTestNow()
	start, end := now.Add(-time.Minute), now
	ctx := context.Background()
	c := tierContextFor("acme/model-a", true, true)

	t.Run("store cannot attribute models", func(t *testing.T) {
		svc := &providerTierLimitService{store: &fakeTierStore{events: 99}}
		got, err := svc.readMinuteRequestCount(ctx, c, start, end)
		if !errors.Is(err, errModelAttributionUnavailable) || got != 0 {
			t.Fatalf("got %v err %v", got, err)
		}
	})
	t.Run("recorded request without a model", func(t *testing.T) {
		store := &fakeModelTierStore{groups: []sqliterepo.ManualModelWindowUsage{{Model: " ", Usage: sqliterepo.ManualWindowUsage{EventCount: 1}}}}
		svc := &providerTierLimitService{store: store}
		if _, err := svc.readMinuteRequestCount(ctx, c, start, end); !errors.Is(err, errModelAttributionUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("live request without a model", func(t *testing.T) {
		svc := &providerTierLimitService{store: &fakeModelTierStore{}}
		svc.requests.recordForModel("p1", "k1", "", now.Add(-5*time.Second))
		if _, err := svc.readMinuteRequestCount(ctx, c, start, end); !errors.Is(err, errModelAttributionUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("live request with conflicting models", func(t *testing.T) {
		svc := &providerTierLimitService{store: &fakeModelTierStore{}}
		at := now.Add(-5 * time.Second)
		svc.requests.recordForModel("p1", "k1", "acme/model-a", at)
		svc.requests.recordForModel("p1", "k1", "acme/model-b", at)
		if _, err := svc.readMinuteRequestCount(ctx, c, start, end); !errors.Is(err, errModelAttributionUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no profiles", func(t *testing.T) {
		noIDs := c
		noIDs.ids = nil
		svc := &providerTierLimitService{store: &fakeModelTierStore{}}
		got, err := svc.readMinuteRequestCount(ctx, noIDs, start, end)
		if err != nil || got != 0 {
			t.Fatalf("got %v err %v", got, err)
		}
	})
}

func TestProvenAccountLimitUsesOnlyAccountWideEvidence(t *testing.T) {
	now := tierTestNow()
	limits := agentusage.TierLimits{
		RequestsPerModel: true, RequestsPerMinute: tierF(1), RequestsPerWindow: tierF(1), RequestWindowSeconds: 60,
		RequestsPerDay: tierF(10),
		Windows: []agentusage.LimitBudget{
			{Window: agentusage.WindowPeriodWeek, Unit: "tokens", Value: tierF(5), Reset: "rolling", Scope: string(agentusage.WindowScopeFreeModels)},
		},
		Scopes: []agentusage.RequestScope{{ID: agentusage.ModelScopeID("a"), Models: []string{"a"}, RequestsPerDay: tierF(1)}},
	}
	used := agentusage.LimitUsage{
		RequestsMinute: 50, RequestsWindow: 50, RequestsDay: 9,
		Windows: map[string]float64{agentusage.WindowPeriodWeek + ":" + string(agentusage.WindowScopeFreeModels): 99},
		Scopes:  map[string]float64{agentusage.ModelScopeID("a"): 99},
	}
	if got := provenAccountLimit(limits, used, now); got.Reason != "" {
		t.Fatalf("unproven per-model, scoped and scope usage must not block: %+v", got)
	}
	used.RequestsDay = 10
	if got := provenAccountLimit(limits, used, now); got.Reason != "limit_daily" {
		t.Fatalf("account-wide daily evidence must still block: %+v", got)
	}
}
