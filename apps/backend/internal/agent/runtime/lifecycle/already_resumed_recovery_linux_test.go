//go:build linux

package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

type loadedNativeDeliveryReconciler interface {
	ReconcileAlreadyResumedDelivery(context.Context, AgentDeliveryRecoveryIdentity, string) (bool, error)
}

func TestRestartRecoveryRetiresOnlyProvenDeadLoadedConversation(t *testing.T) {
	for _, condition := range []string{"dead owner", "live owner", "missing proof", "different native", "active replacement"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			child := exec.Command("/bin/sleep", "60")
			child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			require.NoError(t, child.Start())
			t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
			birth, err := processidentity.Capture(child.Process.Pid)
			require.NoError(t, err)
			if condition != "live owner" {
				require.NoError(t, child.Process.Kill())
				require.Error(t, child.Wait())
			}
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation",
				HarnessGeneration: 2, StreamID: "stream", Hash: "hash", State: journal.SubmissionInterruptedUnknown}
			_, err = store.PutSubmission(ctx, peer)
			require.NoError(t, err)
			manager := newTestManager(t)
			manager.SetAgentDeliveryRepository(&adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{
				peer.ID: {ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID,
					HarnessGeneration: 2, PayloadHash: peer.Hash, State: models.DeliverySubmissionInterruptedUnknown}}})
			replacement := &AgentExecution{ID: "replacement", TaskID: "task", SessionID: "session", ACPSessionID: "native",
				Status: v1.AgentStatusReady, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation",
				DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", agentctl: nativeResumeJournalPeer(t, store, false)}
			replacement.setSessionInitialized(true)
			if condition == "different native" {
				replacement.ACPSessionID = "foreign"
			}
			if condition == "active replacement" {
				replacement.dispatchedPromptPending.Store(true)
			}
			require.NoError(t, manager.executionStore.Add(replacement))
			identity := AgentDeliveryRecoveryIdentity{TaskID: "task", SessionID: "session", ExecutionID: "old",
				IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", SubmissionID: peer.ID,
				PromptGeneration: 3, OriginalRuntime: birth}
			if condition == "missing proof" {
				identity.OriginalRuntime = processidentity.Identity{}
			}
			reconciler, supported := any(manager).(loadedNativeDeliveryReconciler)
			require.True(t, supported, "restart recovery must finish an already loaded native successor")
			restored, recoverErr := reconciler.ReconcileAlreadyResumedDelivery(ctx, identity, "native")
			if condition == "dead owner" {
				require.NoError(t, recoverErr)
				require.True(t, restored)
			} else {
				require.Error(t, recoverErr)
				require.False(t, restored)
			}
			saved, err := store.GetSubmission(ctx, peer.ID)
			require.NoError(t, err)
			require.Equal(t, condition == "dead owner", saved.Retired)
			require.Equal(t, journal.SubmissionInterruptedUnknown, saved.State)
			current, exists := manager.GetExecutionBySessionID("session")
			require.True(t, exists)
			require.Same(t, replacement, current)
		})
	}
}
