package backendapp

import (
	"context"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

type missingReplayProfileStore struct{ settingsstore.Repository }

func (missingReplayProfileStore) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return nil, nil
}

func TestProductionDreamReplayWiringStartsAndSettlesWithoutNilClock(t *testing.T) {
	p, _, convTask, setter := runLedgerWiringWithSetter(t, true)
	p.services = &Services{Pricing: fakeModelPrices{}}
	p.agentSettingsRepo = missingReplayProfileStore{}
	id, found, err := setter.svc.CoordinatorForConversationTask(t.Context(), convTask)
	if err != nil || !found {
		t.Fatalf("coordinator missing: %v %v", found, err)
	}
	result, err := newCoordinatorReplay(p, setter.svc).Run(t.Context(), replay.Request{CoordinatorID: id, Candidate: replay.Candidate{Kind: replay.KindNoteAdd, Text: "candidate"}})
	if err != nil {
		t.Fatalf("production replay dependencies did not start: %v", err)
	}
	if result.RowID == "" || result.Verdict != replay.VerdictUnmeasured || result.Reason != replay.ReasonReadFailed {
		t.Fatalf("missing profile was not an unmeasured result: %+v", result)
	}
	var status string
	err = p.dbPool.Reader().GetContext(t.Context(), &status, p.dbPool.Reader().Rebind(`SELECT status FROM coordinator_replay_results WHERE id = ?`), result.RowID)
	if err != nil || status != replay.StatusDone {
		t.Fatalf("production result did not settle: %s %v", status, err)
	}
}
