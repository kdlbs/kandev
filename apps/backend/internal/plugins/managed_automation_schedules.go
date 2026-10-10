package plugins

import (
	"context"
	"errors"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

var (
	ErrManagedScheduleNotFound         = errors.New("plugins: managed schedule not found")
	ErrManagedScheduleRevisionConflict = errors.New("plugins: managed schedule revision conflict")
	ErrManagedScheduleInvalid          = errors.New("plugins: managed schedule is invalid")
)

// ManagedConversationScheduleService is the automation-service slice used by
// exact Host schedule operations. Installation and plugin identities are
// always supplied by the bound Host, never accepted from an RPC identity.
type ManagedConversationScheduleService interface {
	ListManagedConversationSchedules(ctx context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedConversationSchedule, error)
	CreateManagedConversationSchedule(ctx context.Context, installationID, pluginID string, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error)
	UpdateManagedConversationSchedule(ctx context.Context, installationID, pluginID, workspaceID, automationID string, expectedRevision uint64, schedule pluginsdk.ManagedConversationSchedule, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error)
	SetManagedConversationScheduleEnabled(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, enabled bool, operationID, payloadDigest string) (pluginsdk.ManagedConversationSchedule, bool, error)
	DeleteManagedConversationSchedule(ctx context.Context, installationID, workspaceID, automationID string, expectedRevision uint64, operationID, payloadDigest string) (bool, error)
}
