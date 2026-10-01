package backendapp

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/dream"
	"github.com/kandev/kandev/internal/coordinator/replay"
	replaywire "github.com/kandev/kandev/internal/coordinator/replay/wire"
)

// dreamAgreement serves the rated-agreement measure from the dream tables.
type dreamAgreement struct{ store *coordinator.Store }

func (a dreamAgreement) Agreement(ctx context.Context, coordinatorID string, since, until time.Time) (int64, int64, error) {
	return a.store.DreamAgreement(ctx, coordinatorID, since, until)
}

// dreamConditions are the admission inputs that live outside the store.
type dreamConditions struct {
	svc    *coordinator.Service
	models profileModels
}

func (d dreamConditions) ContainmentOK(ctx context.Context, c *coordinator.Coordinator) bool {
	ck := d.svc.Containment()
	return ck != nil && ck.Check(ctx, c).Contained
}

func (d dreamConditions) Spend(ctx context.Context, c *coordinator.Coordinator, now time.Time) (measurable, atCeiling bool) {
	reading, err := d.svc.Spend(ctx, c, now)
	if !coordinator.CheckSpendMeasurable(reading, err) {
		return false, false
	}
	if c.CostCeilingSubcents == nil {
		return true, true
	}
	return true, !coordinator.CheckCeilingNotReached(reading, *c.CostCeilingSubcents)
}

func (d dreamConditions) Model(ctx context.Context, c *coordinator.Coordinator) string {
	return d.models.model(ctx, c.AgentProfileID)
}

type profileModels struct{ profiles replaywire.ProfileReader }

func (m profileModels) model(ctx context.Context, profileID string) string {
	prof, err := m.profiles.GetAgentProfile(ctx, profileID)
	if err != nil || prof == nil {
		return ""
	}
	return prof.Model
}

// wireCoordinatorDream starts the shadow dream when phase 3.1 is on. A missing
// dependency leaves the dream off rather than half-built. It must run before
// the wake backstop starts.
func wireCoordinatorDream(p routeParams, svc *coordinator.Service) {
	if !svc.Phase31Enabled() || p.taskSvc == nil || p.orchestratorSvc == nil ||
		p.hostUtilityMgr == nil || p.agentSettingsRepo == nil || p.services.Pricing == nil {
		return
	}
	store := svc.Store()
	harness := replay.New(replay.Deps{
		Cases:        coordinator.NewReplayCases(store),
		Profiles:     replaywire.Profiles{Store: store, Profiles: p.agentSettingsRepo},
		Prompts:      replaywire.Prompts{Runner: p.hostUtilityMgr},
		Prices:       replaywire.Prices{Source: p.services.Pricing},
		Spend:        replaywire.Spend{Store: store, Source: svc},
		Instructions: replaywire.Instructions{Store: store, Source: svc, WorkspaceName: dreamWorkspaceName(p)},
		Results:      coordinator.NewReplayResults(store),
	})
	sch := dream.New(dream.Deps{
		Store: store,
		Episode: coordinator.DreamEpisode{
			Tasks: p.taskSvc, Sessions: p.orchestratorSvc, Prompter: coordinator.PromptDreamEpisode(p.orchestratorSvc),
		},
		Replay:     harness,
		Conditions: dreamConditions{svc: svc, models: profileModels{profiles: p.agentSettingsRepo}},
		Log:        p.log.Zap(),
	})
	svc.SetBackstopHooks(coordinator.Hooks{Dream: func(ctx context.Context, id string) error {
		sch.Tick(ctx, id)
		return nil
	}})
	svc.SetDreamStop(sch.StopCoordinator)
	svc.SetLearningHealth(sch)
	if p.addCleanup != nil {
		p.addCleanup(func() error {
			svc.SetDreamStop(nil)
			sch.Stop()
			return nil
		})
	}
	p.log.Debug("coordinator shadow dream wired", zap.Bool("replay", true))
}

func dreamWorkspaceName(p routeParams) func(context.Context, string) string {
	return func(ctx context.Context, workspaceID string) string {
		ws, err := p.taskSvc.GetWorkspace(ctx, workspaceID)
		if err != nil || ws == nil {
			return ""
		}
		return ws.Name
	}
}
