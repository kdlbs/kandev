package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/config"
	officestore "github.com/kandev/kandev/internal/office/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/stretchr/testify/require"
)

// @covers AC-ORCHESTRATION-ASSISTANT-010.1, AC-ORCHESTRATION-ASSISTANT-010.3
func TestOrchestratorFeatureGateNativeRunMatrix(t *testing.T) {
	a, _, repo, taskID := privateConversationFixture(t)
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	office, err := officestore.NewWithDB(db, db, nil)
	require.NoError(t, err)
	for _, orchestration := range []bool{false, true} {
		t.Run(fmt.Sprintf("orchestration=%v", orchestration), func(t *testing.T) {
			var features config.FeaturesConfig
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"orchestration":%v}`, orchestration)), &features))
			allowed, err := orchestrationRunGuard(features, office)(context.Background(), "private-chief")
			require.NoError(t, err)
			require.Equal(t, orchestration, allowed)
			owner, err := repo.ConversationUserOwner(context.Background(), taskID)
			require.NoError(t, err)
			require.Equal(t, "owner", owner)
		})
	}
}

func TestAssistantFeatureGateNativeConversationDispatch(t *testing.T) {
	_, tasks, repo, taskID := privateConversationFixture(t)
	task, err := tasks.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	for _, runtime := range []*orchestrationruntime.Service{nil, {Repo: repo}} {
		guard := assistantDispatchGuard(runtime, repo)
		require.ErrorIs(t, guard(context.Background(), task, nil, "profile"), orchestrationruntime.ErrAssistantDisabled)
	}
}
