package automation

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type automationRunInitiatorContextKey struct{}

func withAutomationRunInitiator(ctx context.Context) context.Context {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return ctx
	}
	return context.WithValue(ctx, automationRunInitiatorContextKey{}, identity.UserID)
}

func credentialMutationActor(ctx context.Context, creatorID string) error {
	actorID, ok := ctx.Value(automationRunInitiatorContextKey{}).(string)
	if !ok {
		identity, found := authn.IdentityFromContext(ctx)
		if !found || identity.Synthetic {
			return nil
		}
		actorID = identity.UserID
	}
	if actorID != creatorID {
		return ErrCredentialCreatorRequired
	}
	return nil
}

func (s *Store) migrateExecutorCreators() error {
	migrate := db.NewRequiredMigrateLogger(s.db, nil)
	for _, table := range []string{"automations"} {
		if err := migrate.Apply(table+".created_by_user_id", "ALTER TABLE "+table+" ADD COLUMN created_by_user_id TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return nil
}

// SetExecutorContext installs the current-identity policy before pollers start.
func (s *Service) SetExecutorContext(resolve func(context.Context, string, string) (context.Context, error)) {
	s.executorContext = resolve
}

func (s *Service) executorActorContext(ctx context.Context, creator, profile string, workspace ...string) (context.Context, error) {
	if s.executorContext == nil {
		return ctx, nil
	}
	if len(workspace) > 0 {
		ctx = taskmodels.WithExecutorWorkspace(ctx, workspace[0])
	}
	return s.executorContext(ctx, creator, profile)
}
