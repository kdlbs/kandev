package coordinator

import (
	"context"
	"errors"
	"expvar"
	"fmt"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Outcomes of the automatic path, the closed label set of
// coordinator_automatic_approval_total.
const (
	automaticApproved      = "approved"
	automaticFailed        = "failed"
	automaticLimited       = "limited"
	automaticHeld          = "held"
	automaticRaiserInvalid = "raiser_invalid"
)

var (
	automaticApprovalTotal = expvar.NewMap("coordinator_automatic_approval_total")
	// automaticHeldTotal counts admission holds by the closed admission reason
	// set plus read_error.
	automaticHeldTotal = expvar.NewMap("coordinator_automatic_held_total")
	// automaticLowerFailedTotal counts lowerings that failed, by trigger.
	automaticLowerFailedTotal = expvar.NewMap("coordinator_automatic_lower_failed_total")
)

// Triggers of coordinator_automatic_lower_failed_total.
const (
	lowerTriggerRaiserInvalid = "raiser_invalid"
	lowerTriggerFailure       = "automatic_failure"
	lowerTriggerOverride      = "manager_override"
	lowerTriggerUndo          = "undo"
	lowerTriggerSettings      = "settings"
)

// automaticTurnKey carries the unattended turn an automatic approval written
// inside a propose call belongs to.
type automaticTurnKey struct{}

func withAutomaticTurn(ctx context.Context, turnID *string) context.Context {
	return context.WithValue(ctx, automaticTurnKey{}, turnID)
}

func automaticTurnFrom(ctx context.Context) *string {
	id, _ := ctx.Value(automaticTurnKey{}).(*string)
	return id
}

// AutomaticResult is what propose_task_kandev returns for a proposal the
// automatic path handled. A nil result means the phase 1 result stands.
type AutomaticResult struct {
	Status ProposalStatus
	TaskID string
	Note   string
}

type raiserVerdict int

const (
	raiserValid raiserVerdict = iota
	raiserInvalid
)

// raiserContext returns the context the automatic approval runs under: the
// raising manager's identity, after checking they are an active user who
// still holds workspace.manage. An empty raiser is the auth-disabled case and
// keeps the caller's context. An error means the check could not decide.
func (s *Service) raiserContext(ctx context.Context, workspaceID, raiser string) (context.Context, raiserVerdict, error) {
	if raiser == "" {
		if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
			return ctx, raiserInvalid, raiserCheckError(err)
		}
		return ctx, raiserValid, nil
	}
	resolver := s.identityResolver()
	if resolver == nil {
		return ctx, raiserInvalid, fmt.Errorf("%w: no identity resolver", errRaiserUnavailable)
	}
	identity, ok, err := resolver.ResolveUserIdentity(ctx, raiser)
	if err != nil {
		return ctx, raiserInvalid, fmt.Errorf("%w: %v", errRaiserUnavailable, err)
	}
	if !ok {
		return ctx, raiserInvalid, nil
	}
	raiserCtx := authn.WithIdentity(ctx, identity)
	if err := s.authz.AuthorizeWorkspaceScope(raiserCtx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return ctx, raiserInvalid, raiserCheckError(err)
	}
	return raiserCtx, raiserValid, nil
}

// raiserCheckError maps a refusal to a nil error (the manager no longer
// qualifies) and any other failure to errRaiserUnavailable.
func raiserCheckError(err error) error {
	if errors.Is(err, taskservice.ErrForbidden) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		return nil
	}
	return fmt.Errorf("%w: %v", errRaiserUnavailable, err)
}
