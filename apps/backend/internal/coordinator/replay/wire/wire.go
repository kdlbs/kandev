// Package wire adapts the real coordinator stores, the host utility prompt
// runner, the price lookup and the spend reader to the interfaces of the replay
// harness. It is the only replay package that imports them.
package wire

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/hostutility"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
	"github.com/kandev/kandev/internal/office/shared"
)

// ProfileReader reads an agent profile by id.
type ProfileReader interface {
	GetAgentProfile(ctx context.Context, id string) (*settingsmodels.AgentProfile, error)
}

// Profiles resolves the coordinator's own agent profile.
type Profiles struct {
	Store    *coordinator.Store
	Profiles ProfileReader
}

var _ replay.Profiles = Profiles{}

// Resolve returns the coordinator's profile with its safety-relevant fields.
func (p Profiles) Resolve(ctx context.Context, coordinatorID string) (replay.Profile, error) {
	c, err := p.Store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return replay.Profile{}, fmt.Errorf("replay profile: read coordinator: %w", err)
	}
	prof, err := p.Profiles.GetAgentProfile(ctx, c.AgentProfileID)
	if err != nil {
		return replay.Profile{}, fmt.Errorf("replay profile: read agent profile: %w", err)
	}
	if prof == nil {
		return replay.Profile{}, errors.New("replay profile: agent profile not found")
	}
	out := replay.Profile{ID: prof.ID, Model: prof.Model, AutoApprove: prof.AutoApprove, Prefix: prof.CommandPrefix}
	for _, f := range prof.CLIFlags {
		if f.Enabled {
			out.EnabledFlags = append(out.EnabledFlags, f.Flag)
		}
	}
	return out, nil
}

// PromptRunner is the sessionless prompt surface of the host utility manager.
type PromptRunner interface {
	ExecuteProfilePrompt(ctx context.Context, profileID, prompt string) (*hostutility.PromptResult, error)
}

// Prompts runs one sessionless prompt through the host utility manager.
type Prompts struct{ Runner PromptRunner }

var _ replay.Prompts = Prompts{}

// Run executes the prompt; the model the executor reports is not used.
func (p Prompts) Run(ctx context.Context, profileID, prompt string) (replay.Reply, error) {
	res, err := p.Runner.ExecuteProfilePrompt(ctx, profileID, prompt)
	if err != nil {
		return replay.Reply{}, err
	}
	if res == nil {
		return replay.Reply{}, errors.New("replay prompt: empty result")
	}
	return replay.Reply{Text: res.Response, PromptTokens: int64(res.PromptTokens), ResponseTokens: int64(res.ResponseTokens)}, nil
}

// PriceSource is the model price lookup the usage recorder uses.
type PriceSource interface {
	LookupForModelWithVersion(ctx context.Context, modelID string) (shared.ModelPricing, string, bool)
}

// Prices looks a model's price up.
type Prices struct{ Source PriceSource }

var _ replay.Prices = Prices{}

// Lookup returns the model's price and whether one was found.
func (p Prices) Lookup(ctx context.Context, model string) (commoncosts.ModelPricing, bool, error) {
	price, _, ok := p.Source.LookupForModelWithVersion(ctx, model)
	if !ok {
		return commoncosts.ModelPricing{}, false, nil
	}
	return commoncosts.ModelPricing(price), true, nil
}

// SpendSource is the coordinator service's spend reader.
type SpendSource interface {
	Spend(ctx context.Context, c *coordinator.Coordinator, now time.Time) (coordinator.SpendReading, error)
}

// Spend reads the coordinator's 24 hour spend, replay rows included, and its
// ceiling.
type Spend struct {
	Store  *coordinator.Store
	Source SpendSource
}

var _ replay.Spend = Spend{}

// Reading returns the spend, unmeasurable on any read failure.
func (s Spend) Reading(ctx context.Context, coordinatorID string, now time.Time) (replay.SpendReading, error) {
	c, err := s.Store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return replay.SpendReading{}, fmt.Errorf("replay spend: read coordinator: %w", err)
	}
	r, err := s.Source.Spend(ctx, c, now)
	if err != nil {
		return replay.SpendReading{}, err
	}
	return replay.SpendReading{WindowSubcents: r.WindowSubcents, Measurable: coordinator.CheckSpendMeasurable(r, nil), CeilingSubcents: c.CostCeilingSubcents}, nil
}

// InstructionSource reads the instruction inputs of one coordinator.
type InstructionSource interface {
	GoalInstructionSection(ctx context.Context, coordinatorID string) (string, error)
}

// Instructions renders the baseline and candidate instruction text.
type Instructions struct {
	Store         *coordinator.Store
	Source        InstructionSource
	WorkspaceName func(ctx context.Context, workspaceID string) string
}

var _ replay.Instructions = Instructions{}

// Render reads the context, the active orders and the goal once and renders
// both sides. An unreadable order list is an error, never an orders-less
// baseline.
func (i Instructions) Render(ctx context.Context, coordinatorID string, o replay.Override) (replay.Renders, error) {
	c, err := i.Store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return replay.Renders{}, fmt.Errorf("replay render: read coordinator: %w", err)
	}
	orders, err := i.Store.ActiveStandingOrders(ctx, coordinatorID)
	if err != nil {
		return replay.Renders{}, fmt.Errorf("replay render: read standing orders: %w", err)
	}
	goal, err := i.Source.GoalInstructionSection(ctx, coordinatorID)
	if err != nil {
		return replay.Renders{}, fmt.Errorf("replay render: read goal: %w", err)
	}
	name := ""
	if i.WorkspaceName != nil {
		name = i.WorkspaceName(ctx, c.WorkspaceID)
	}
	return coordinator.ReplayRender(coordinator.ReplayRenderInput{
		WorkspaceName: name, WorkspaceID: c.WorkspaceID, Name: c.Name, Context: c.Context, Orders: orders, GoalSection: goal,
	}, o)
}
