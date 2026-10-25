package controller

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/notifications/dto"
	"github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// @covers AC-PLATFORM-NOTIFICATIONS-001.6, AC-PLATFORM-NOTIFICATIONS-001.10
func TestControllerProviderEventSelection(t *testing.T) {
	for _, body := range []string{`{"name":"Changed"}`, `{"name":"Changed","events":null}`, `{"name":"Changed","events":[]}`} {
		t.Run(body, func(t *testing.T) {
			raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "notifications.db"))
			require.NoError(t, err)
			database := sqlx.NewDb(raw, "sqlite3")
			t.Cleanup(func() { require.NoError(t, database.Close()) })
			repo, _, err := notificationstore.Provide(context.Background(), database, database)
			require.NoError(t, err)
			log, err := logger.NewFromZap(zap.NewNop())
			require.NoError(t, err)
			controller := NewController(service.NewService(repo, nil, nil, log, nil))
			provider, err := controller.CreateProvider(context.Background(), decodeCreateProviderRequest(t, `{"type":"local","events":null}`))
			require.NoError(t, err)
			require.Equal(t, []string{service.EventTaskSessionClarificationAsked}, provider.Events)
			beforeSubs, err := repo.ListSubscriptionsByProvider(context.Background(), provider.ID)
			require.NoError(t, err)
			var request dto.UpdateProviderRequest
			require.NoError(t, json.Unmarshal([]byte(body), &request))
			updated, err := controller.UpdateProvider(context.Background(), provider.ID, request)
			require.NoError(t, err)
			require.Equal(t, "Changed", updated.Name)
			subs, err := repo.ListSubscriptionsByProvider(context.Background(), provider.ID)
			require.NoError(t, err)
			if request.Events == nil {
				require.Equal(t, provider.Events, updated.Events)
				require.Equal(t, beforeSubs, subs)
			} else {
				require.Empty(t, updated.Events)
				require.Empty(t, subs)
			}
			persisted, err := repo.GetProvider(context.Background(), userstore.DefaultUserID, provider.ID)
			require.NoError(t, err)
			require.Equal(t, "Changed", persisted.Name)
		})
	}
}
