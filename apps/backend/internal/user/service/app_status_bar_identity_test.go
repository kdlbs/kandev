package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
	"go.uber.org/zap"
)

func TestAppStatusBarPreferenceUsesAuthenticatedUser(t *testing.T) {
	const userID = "member-1"
	repo := &recordingUserRepository{
		getSettings: &models.UserSettings{UserID: userID, AppStatusBarEnabled: true},
		preservingSettings: &models.UserSettings{
			UserID:              userID,
			AppStatusBarEnabled: false,
		},
	}
	eventBus := &recordingEventBus{}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: userID,
		Role:   authn.RoleMember,
	})
	svc := NewService(repo, eventBus, log)

	settings, err := svc.GetUserSettings(ctx)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if repo.getSettingsUserID != userID || settings.UserID != userID {
		t.Fatalf("GetUserSettings user = %q, settings user = %q, want %q", repo.getSettingsUserID, settings.UserID, userID)
	}

	disabled := false
	if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{AppStatusBarEnabled: &disabled}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if repo.getSettingsUserID != userID {
		t.Fatalf("UpdateUserSettings read user = %q, want %q", repo.getSettingsUserID, userID)
	}
	if repo.preservingInput == nil || repo.preservingInput.UserID != userID {
		t.Fatalf("persisted settings user = %+v, want %q", repo.preservingInput, userID)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("published events = %d, want 1", len(eventBus.publishedEvents))
	}
	data, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok || data["user_id"] != userID {
		t.Fatalf("published user = %#v, want %q", data["user_id"], userID)
	}
}

func TestSettingsUserIDFallsBackOnlyForUnscopedCallers(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	svc := NewService(&recordingUserRepository{}, nil, log)
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "internal", ctx: context.Background(), want: "default-user"},
		{
			name: "synthetic",
			ctx: authn.WithIdentity(context.Background(), authn.Identity{
				UserID:    "ignored-synthetic-user",
				Role:      authn.RoleAdmin,
				Synthetic: true,
			}),
			want: "default-user",
		},
		{
			name: "first real user",
			ctx:  authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1"}),
			want: "member-1",
		},
		{
			name: "second real user",
			ctx:  authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-2"}),
			want: "member-2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.settingsUserID(tt.ctx); got != tt.want {
				t.Fatalf("settingsUserID = %q, want %q", got, tt.want)
			}
		})
	}
}
