package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

func TestUpdateUserSettingsPersistsTurnChangedFilesFalse(t *testing.T) {
	updated := &models.UserSettings{UserID: store.DefaultUserID, Revision: 2}
	eventBus := &recordingEventBus{}
	repo := &recordingUserRepository{
		getSettings:        &models.UserSettings{UserID: store.DefaultUserID, ShowTurnChangedFiles: true, Revision: 1},
		preservingSettings: updated,
	}
	var _ bus.EventBus = eventBus
	disabled := false
	got, err := newCASService(repo, eventBus).UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		ShowTurnChangedFiles: &disabled,
	})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if repo.preservingInput == nil || repo.preservingInput.ShowTurnChangedFiles {
		t.Fatalf("persisted preference = %+v, want false", repo.preservingInput)
	}
	if got.ShowTurnChangedFiles {
		t.Fatal("updated settings should keep explicit false")
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("published settings events = %d, want 1", len(eventBus.publishedEvents))
	}
	data, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok || data["show_turn_changed_files"] != false {
		t.Fatalf("event preference = %#v, want false", data["show_turn_changed_files"])
	}
}

func TestResolveTurnChangedFilesCapturePolicyUsesSettingsIdentity(t *testing.T) {
	settingsErr := errors.New("settings unavailable")
	tests := []struct {
		name     string
		ctx      context.Context
		repo     *recordingUserRepository
		userID   string
		kind     TurnChangedFilesPolicyResolutionKind
		enabled  bool
		revision int64
		wantErr  error
	}{
		{
			name:   "authenticated initiator",
			ctx:    authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1"}),
			repo:   &recordingUserRepository{getSettings: &models.UserSettings{ShowTurnChangedFiles: true, Revision: 12}},
			userID: "member-1", kind: TurnChangedFilesPolicyAuthenticatedUser, enabled: true, revision: 12,
		},
		{
			name:   "internal context uses default settings user",
			ctx:    context.Background(),
			repo:   &recordingUserRepository{getSettings: &models.UserSettings{ShowTurnChangedFiles: false, Revision: 7}},
			userID: "default-user", kind: TurnChangedFilesPolicyDefaultUser, enabled: false, revision: 7,
		},
		{
			name: "synthetic context keeps default settings behavior",
			ctx: authn.WithIdentity(context.Background(), authn.Identity{
				UserID: "synthetic-actor", Synthetic: true,
			}),
			repo:   &recordingUserRepository{getSettings: &models.UserSettings{ShowTurnChangedFiles: true, Revision: 3}},
			userID: "default-user", kind: TurnChangedFilesPolicySyntheticDefault, enabled: true, revision: 3,
		},
		{
			name:   "read failure disables capture",
			ctx:    authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-2"}),
			repo:   &recordingUserRepository{getErr: settingsErr},
			userID: "member-2", kind: TurnChangedFilesPolicyReadFailed, wantErr: settingsErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := newCASService(test.repo, nil).ResolveTurnChangedFilesCapturePolicy(test.ctx)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.repo.getSettingsUserID != test.userID || policy.SettingsUserID != test.userID {
				t.Fatalf("settings user = %q (read %q), want %q", policy.SettingsUserID, test.repo.getSettingsUserID, test.userID)
			}
			if policy.ResolutionKind != test.kind || policy.Enabled != test.enabled || policy.Revision != test.revision {
				t.Fatalf("policy = %+v, want kind=%q enabled=%t revision=%d", policy, test.kind, test.enabled, test.revision)
			}
		})
	}
}
