package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ac76GuardedSourceFiles are the two files that make up the entire
// turn-completion notification path AC-76 protects: the publisher
// (task/service.CompleteTurn / AbandonOpenTurns -> publishTurnEvent, which
// emits events.TurnCompleted) and the consumer (this package's
// handleSemanticOccurrence, reached via HandleTaskTurnFinished from the
// events.TurnCompleted subscription in backendapp/gateway.go). Neither may
// reference anything the parked-on-background-work slice introduces.
var ac76GuardedSourceFiles = []string{
	"service.go",                          // this package: handleSemanticOccurrence and friends
	"../../task/service/service_turns.go", // CompleteTurn / AbandonOpenTurns / publishTurnEvent
}

// TestTurnCompletionPathNeverReferencesParkedProjection is AC-76's structural
// guard: the parked-on-background-work slice lives entirely in
// orchestrator.updateTaskSessionSessionWithHook (session state transitions)
// and orchestrator.handleToolCallEvent/trackBackgroundToolUpdate (tool call
// attestation) — neither of which is on the turn-completion path. This test
// asserts that invariant at the source level: the two files that make up the
// entire turn-completion -> notification pipeline never mention "parked" in
// any casing, so nothing this slice introduces can withhold, defer, delay,
// reorder, or drop a turn-finished notification.
func TestTurnCompletionPathNeverReferencesParkedProjection(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(sourceFile)

	var violations []string
	for _, rel := range ac76GuardedSourceFiles {
		path := filepath.Join(dir, rel)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "read %s", rel)
		if strings.Contains(strings.ToLower(string(data)), "parked") {
			violations = append(violations, rel+" references \"parked\"")
		}
	}
	require.Empty(t, violations, "turn-completion notification path must stay untouched by the parked-projection slice (AC-76):\n%s", strings.Join(violations, "\n"))
}

// TestHandleTaskTurnFinished_ByteIdenticalAcrossParkedCases is AC-76's
// behavioural clause: Service.handleSemanticOccurrence produces a
// byte-identical notificationPayload shape and exactly one InsertDelivery per
// occurrence, regardless of whether the originating session was parked,
// un-parked, had an unknown probe result, or was never attested at all. The
// notification service takes no parked-state input of any kind — these four
// case labels exist only to document the AC's named scenarios; the service
// call is identical in every case, which is itself the proof that parked
// state cannot reach this path.
func TestHandleTaskTurnFinished_ByteIdenticalAcrossParkedCases(t *testing.T) {
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)

	cases := []struct {
		name      string
		taskID    string
		sessionID string
		turnID    string
	}{
		{"parked", "task-parked", "session-parked", "turn-parked"},
		{"un-parked", "task-unparked", "session-unparked", "turn-unparked"},
		{"unknown-probe", "task-unknown", "session-unknown", "turn-unknown"},
		{"no-recogniser", "task-norecog", "session-norecog", "turn-norecog"},
	}

	type observed struct {
		eventType     string
		title         string
		body          string
		taskID        string
		taskSessionID string
		occurrenceID  string
	}
	var fixture *observed

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &notificationTestRepository{
				providers: []*models.Provider{{ID: "provider-1", Type: models.ProviderTypeLocal, Enabled: true}},
				subscriptions: map[string][]*models.Subscription{
					"provider-1": {
						{ProviderID: "provider-1", EventType: EventTaskSessionTurnFinished, Enabled: true},
					},
				},
			}
			svc := NewService(repo, notificationTestTaskGetter{task: &taskmodels.Task{Title: "Fix delivery"}}, nil, log)
			capture := &captureProvider{}
			svc.providers[models.ProviderTypeLocal] = capture

			svc.HandleTaskTurnFinished(context.Background(), tc.taskID, tc.sessionID, tc.turnID)

			require.Len(t, capture.messages, 1, "case %q: expected exactly one notification dispatch", tc.name)
			require.Len(t, repo.deliveries, 1, "case %q: expected exactly one InsertDelivery", tc.name)
			require.Equal(t, tc.turnID, repo.deliveries[0].OccurrenceID, "case %q: InsertDelivery occurrence id", tc.name)

			got := &observed{
				eventType:     capture.messages[0].EventType,
				title:         capture.messages[0].Title,
				body:          capture.messages[0].Body,
				taskID:        capture.messages[0].TaskID,
				taskSessionID: capture.messages[0].TaskSessionID,
				occurrenceID:  capture.messages[0].OccurrenceID,
			}
			require.Equal(t, tc.taskID, got.taskID, "case %q", tc.name)
			require.Equal(t, tc.sessionID, got.taskSessionID, "case %q", tc.name)
			require.Equal(t, tc.turnID, got.occurrenceID, "case %q", tc.name)

			if fixture == nil {
				fixture = &observed{
					eventType: got.eventType,
					title:     got.title,
					body:      got.body,
				}
				return
			}
			// The event-type/title/body shape (everything not derived
			// directly from this case's own taskID/sessionID/turnID inputs)
			// must be byte-identical to the first case's fixture.
			require.Equal(t, fixture.eventType, got.eventType, "case %q: eventType diverged from fixture", tc.name)
			require.Equal(t, fixture.title, got.title, "case %q: title diverged from fixture", tc.name)
			require.Equal(t, fixture.body, got.body, "case %q: body diverged from fixture", tc.name)
		})
	}
}
