package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/shared"
)

type fakeModelPrices struct{}

func (fakeModelPrices) LookupForModelWithVersion(context.Context, string) (shared.ModelPricing, string, bool) {
	return shared.ModelPricing{}, "", false
}

func TestWireCoordinatorDream_MissingDependencyLeavesTheDreamOff(t *testing.T) {
	p, _, _, setter := runLedgerWiringWithSetter(t, true)
	var cleanups []func() error
	p.addCleanup = func(fn func() error) { cleanups = append(cleanups, fn) }
	p.services = &Services{Pricing: fakeModelPrices{}}
	wireCoordinatorDream(p, setter.svc)
	if len(cleanups) != 0 {
		t.Fatalf("wired %d cleanups without an orchestrator", len(cleanups))
	}
}

func TestWireCoordinatorDream_PhaseOffLeavesTheDreamOff(t *testing.T) {
	p, _, _, setter := runLedgerWiringPhases(t, true, false)
	var cleanups []func() error
	p.addCleanup = func(fn func() error) { cleanups = append(cleanups, fn) }
	wireCoordinatorDream(p, setter.svc)
	if len(cleanups) != 0 {
		t.Fatal("dream wired with phase 3.1 off")
	}
}

type fakeProfiles struct {
	prof *settingsmodels.AgentProfile
	err  error
}

func (f fakeProfiles) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return f.prof, f.err
}

func TestProfileModels(t *testing.T) {
	ctx := context.Background()
	if got := (profileModels{fakeProfiles{prof: &settingsmodels.AgentProfile{Model: "m1"}}}).model(ctx, "a"); got != "m1" {
		t.Fatalf("model = %q", got)
	}
	if got := (profileModels{fakeProfiles{err: errors.New("x")}}).model(ctx, "a"); got != "" {
		t.Fatalf("model on error = %q", got)
	}
	if got := (profileModels{fakeProfiles{}}).model(ctx, "a"); got != "" {
		t.Fatalf("model of a missing profile = %q", got)
	}
}

func TestDreamAgreement_ReadsTheDreamTables(t *testing.T) {
	_, _, _, setter := runLedgerWiringWithSetter(t, true)
	rated, agreeing, err := dreamAgreement{store: setter.svc.Store()}.Agreement(context.Background(), "none", time.Now().Add(-time.Hour), time.Now())
	if err != nil || rated != 0 || agreeing != 0 {
		t.Fatalf("agreement = %d, %d, %v", rated, agreeing, err)
	}
}
