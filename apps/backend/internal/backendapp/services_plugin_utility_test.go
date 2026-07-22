package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
)

type pluginUtilityAgentRepository struct {
	utilitystore.Repository
	err error
}

func (r *pluginUtilityAgentRepository) GetAgentByID(context.Context, string) (*utilitymodels.UtilityAgent, error) {
	return nil, r.err
}

func TestPluginsUtilityAgentAdapter_MapsTypedNotFound(t *testing.T) {
	adapter := pluginsUtilityAgentAdapter{
		svc: utilityservice.NewService(&pluginUtilityAgentRepository{err: sql.ErrNoRows}),
	}

	_, err := adapter.GetAgentByID(context.Background(), "missing")
	if !errors.Is(err, plugins.ErrUtilityAgentNotFound) {
		t.Fatalf("GetAgentByID() error = %v, want plugin not-found error", err)
	}
}

func TestPluginsUtilityAgentAdapter_PreservesOperationalFailure(t *testing.T) {
	storeErr := errors.New("utility agent database unavailable")
	adapter := pluginsUtilityAgentAdapter{
		svc: utilityservice.NewService(&pluginUtilityAgentRepository{err: storeErr}),
	}

	_, err := adapter.GetAgentByID(context.Background(), "configured")
	if !errors.Is(err, storeErr) {
		t.Fatalf("GetAgentByID() error = %v, want store error", err)
	}
}
