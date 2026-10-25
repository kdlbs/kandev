package service

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	eventtypes "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

type recentUseRepository struct {
	store.Repository
	records       map[models.AgentProfileRecentUseContext]*models.AgentProfileRecentUse
	upsertCalls   int
	conflictsLeft int
}

func (r *recentUseRepository) GetAgentProfileRecentUse(
	_ context.Context,
	_ string,
	contextValue models.AgentProfileRecentUseContext,
) (*models.AgentProfileRecentUse, error) {
	record := r.records[contextValue]
	if record == nil {
		return nil, sql.ErrNoRows
	}
	copy := *record
	copy.ProfileIDs = append([]string(nil), record.ProfileIDs...)
	return &copy, nil
}

func (r *recentUseRepository) ListAgentProfileRecentUse(
	_ context.Context,
	_ string,
) ([]*models.AgentProfileRecentUse, error) {
	records := make([]*models.AgentProfileRecentUse, 0, len(r.records))
	for _, record := range r.records {
		copy := *record
		copy.ProfileIDs = append([]string(nil), record.ProfileIDs...)
		records = append(records, &copy)
	}
	return records, nil
}

func (r *recentUseRepository) UpsertAgentProfileRecentUse(
	_ context.Context,
	record *models.AgentProfileRecentUse,
	expectedRevision int64,
) (*models.AgentProfileRecentUse, error) {
	r.upsertCalls++
	if r.conflictsLeft > 0 {
		r.conflictsLeft--
		return nil, store.ErrAgentProfileRecentUseRevisionConflict
	}
	if current := r.records[record.Context]; current != nil && current.Revision != expectedRevision {
		return nil, store.ErrAgentProfileRecentUseRevisionConflict
	}
	copy := *record
	copy.ProfileIDs = append([]string(nil), record.ProfileIDs...)
	copy.UpdatedAt = time.Unix(int64(r.upsertCalls), 0).UTC()
	r.records[record.Context] = &copy
	return &copy, nil
}

func newRecentUseService(t *testing.T, repo *recentUseRepository, eventBus *recordingEventBus) *Service {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	return NewService(repo, eventBus, log)
}

func TestRecordAgentProfileRecentUseMovesToFrontAndSuppressesNoop(t *testing.T) {
	repo := &recentUseRepository{records: make(map[models.AgentProfileRecentUseContext]*models.AgentProfileRecentUse)}
	events := &recordingEventBus{}
	svc := newRecentUseService(t, repo, events)
	ctx := context.Background()
	contextValue := models.AgentProfileRecentUseTaskCreate

	for _, profileID := range []string{"profile-a", "profile-b", "profile-c"} {
		if _, err := svc.RecordAgentProfileRecentUse(ctx, contextValue, profileID); err != nil {
			t.Fatalf("record %q: %v", profileID, err)
		}
	}
	if _, err := svc.RecordAgentProfileRecentUse(ctx, contextValue, "profile-b"); err != nil {
		t.Fatalf("move profile-b: %v", err)
	}
	if _, err := svc.RecordAgentProfileRecentUse(ctx, contextValue, "profile-b"); err != nil {
		t.Fatalf("repeat profile-b: %v", err)
	}

	want := []string{"profile-b", "profile-c", "profile-a"}
	if got := repo.records[contextValue].ProfileIDs; !reflect.DeepEqual(got, want) {
		t.Fatalf("profile order = %v, want %v", got, want)
	}
	if repo.upsertCalls != 4 {
		t.Fatalf("upsert calls = %d, want 4 after no-op suppression", repo.upsertCalls)
	}
	if len(events.publishedEvents) != 4 {
		t.Fatalf("published events = %d, want 4", len(events.publishedEvents))
	}
	if events.publishedSubjects[0] != eventtypes.UserAgentProfileRecentUseUpdated {
		t.Fatalf("event subject = %q, want %q", events.publishedSubjects[0], eventtypes.UserAgentProfileRecentUseUpdated)
	}
}

func TestRecordAgentProfileRecentUseCapsHistoryAndRetriesConflicts(t *testing.T) {
	repo := &recentUseRepository{
		records:       make(map[models.AgentProfileRecentUseContext]*models.AgentProfileRecentUse),
		conflictsLeft: 1,
	}
	svc := newRecentUseService(t, repo, &recordingEventBus{})
	for index := 0; index < models.AgentProfileRecentUseMaxProfiles+2; index++ {
		profileID := "profile-" + string(rune('a'+index))
		if _, err := svc.RecordAgentProfileRecentUse(context.Background(), models.AgentProfileRecentUseQuickChat, profileID); err != nil {
			t.Fatalf("record %q: %v", profileID, err)
		}
	}

	record := repo.records[models.AgentProfileRecentUseQuickChat]
	if len(record.ProfileIDs) != models.AgentProfileRecentUseMaxProfiles {
		t.Fatalf("profile count = %d, want %d", len(record.ProfileIDs), models.AgentProfileRecentUseMaxProfiles)
	}
	if record.ProfileIDs[0] != "profile-l" {
		t.Fatalf("leading profile = %q, want profile-l", record.ProfileIDs[0])
	}
	if repo.upsertCalls != models.AgentProfileRecentUseMaxProfiles+3 {
		t.Fatalf("upsert calls = %d, want one conflict retry", repo.upsertCalls)
	}
}

func TestRecordAgentProfileRecentUseValidatesContextAndID(t *testing.T) {
	repo := &recentUseRepository{records: make(map[models.AgentProfileRecentUseContext]*models.AgentProfileRecentUse)}
	svc := newRecentUseService(t, repo, &recordingEventBus{})
	tests := []struct {
		name      string
		context   models.AgentProfileRecentUseContext
		profileID string
	}{
		{name: "unknown context", context: "other", profileID: "profile-a"},
		{name: "empty id", context: models.AgentProfileRecentUseTaskSession, profileID: ""},
		{name: "long id", context: models.AgentProfileRecentUseTaskSession, profileID: string(make([]byte, models.AgentProfileRecentUseMaxProfileIDBytes+1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.RecordAgentProfileRecentUse(context.Background(), tt.context, tt.profileID); !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want validation error", err)
			}
		})
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("upsert calls = %d, want 0 for invalid input", repo.upsertCalls)
	}
}
