package backendapp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.4
func TestGitHubIssueMutationWriterCompatibility(t *testing.T) {
	for _, writer := range []string{"human_title", "generated_title", "priority", "server_set", "server_remove"} {
		for _, issueLast := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s_issue_last_%t", writer, issueLast), func(t *testing.T) {
				h := newIssueMutationHarness(t, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				task, err := h.repos[0].GetTask(ctx, "issue-task")
				require.NoError(t, err)
				task.Metadata = issueMutationMetadata(7)
				task.Metadata[models.MetaKeyAgentTitlePending] = true
				task.Metadata[models.MetaKeyDeferredLaunch] = map[string]interface{}{"prompt": "old"}
				require.NoError(t, h.repos[0].UpdateTask(ctx, task))
				require.NoError(t, h.repos[0].SetTaskMetadataKey(ctx, task.ID, models.MetaKeyAgentTitlePending, true))
				require.NoError(t, h.repos[0].SetTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch, map[string]interface{}{"prompt": "old"}))
				gh, gate := issueMutationGitHub(t, h.services[0])
				write := func() { issueMutationOtherWriter(t, h, ctx, writer) }
				if issueLast {
					gate.armed.Store(true)
					result := make(chan error, 1)
					go func() { result <- runIssueMutation(ctx, gh, 42) }()
					joined := false
					defer func() {
						cancel()
						if !joined {
							<-result
						}
					}()
					select {
					case <-gate.arrived:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					write()
					close(gate.release)
					resultErr := <-result
					joined = true
					require.NoError(t, resultErr)
				} else {
					require.NoError(t, runIssueMutation(ctx, gh, 42))
					write()
				}
				stored, err := h.repos[1].GetTask(ctx, "issue-task")
				require.NoError(t, err)
				assertIssueMutationIdentity(t, stored.Metadata, 42)
				require.Equal(t, "untouched", stored.Metadata["keep"])
				switch writer {
				case "human_title", "generated_title":
					require.Equal(t, "Accepted title", stored.Title)
					require.NotContains(t, stored.Metadata, models.MetaKeyAgentTitlePending)
					require.NotContains(t, stored.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
				case "priority":
					require.Equal(t, "high", stored.Priority)
					require.Equal(t, true, stored.Metadata[models.MetaKeyAgentTitlePending])
				case "server_set":
					for _, key := range issueMutationOwnerKeys() {
						require.Equal(t, map[string]interface{}{"current": true}, stored.Metadata[key])
					}
				case "server_remove":
					for _, key := range issueMutationOwnerKeys() {
						require.NotContains(t, stored.Metadata, key)
					}
				}
			})
		}
	}
}
func issueMutationOwnerKeys() []string {
	return []string{models.MetaKeyDeferredLaunch, models.MetaKeyStepHandoffCarry, "handoff_source", "handoffs", "office_carrier_causation_depth", "workspace"}
}
func issueMutationOtherWriter(t *testing.T, h *issueMutationHarness, ctx context.Context, writer string) {
	t.Helper()
	title, priority := "Accepted title", "high"
	switch writer {
	case "human_title":
		_, err := h.services[1].UpdateTask(ctx, "issue-task", &taskservice.UpdateTaskRequest{Title: &title})
		require.NoError(t, err)
	case "priority":
		_, err := h.services[1].UpdateTask(ctx, "issue-task", &taskservice.UpdateTaskRequest{Priority: &priority})
		require.NoError(t, err)
	case "generated_title":
		claimed, newClaim, err := h.repos[1].ClaimTaskTitleSession(ctx, "issue-task", "owner")
		require.NoError(t, err)
		require.True(t, claimed)
		require.True(t, newClaim)
		wrong, err := h.repos[1].SetTaskTitleIfPending(ctx, "issue-task", "other", title)
		require.NoError(t, err)
		require.False(t, wrong)
		accepted, err := h.repos[1].SetTaskTitleIfPending(ctx, "issue-task", "owner", title)
		require.NoError(t, err)
		require.True(t, accepted)
	case "server_set", "server_remove":
		for _, key := range issueMutationOwnerKeys() {
			require.NoError(t, h.repos[1].SetTaskMetadataKey(ctx, "issue-task", key, map[string]interface{}{"current": true}))
			if writer == "server_remove" {
				removed, err := h.repos[1].RemoveTaskMetadataKey(ctx, "issue-task", key)
				require.NoError(t, err)
				require.True(t, removed)
			}
		}
	}
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.4
func TestGitHubIssueMutationReplacementExclusions(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(fmt.Sprint(snapshot), func(t *testing.T) {
			h := newIssueMutationHarness(t, nil)
			ctx := context.Background()
			stale, err := h.repos[1].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			gh, _ := issueMutationGitHub(t, h.services[0])
			require.NoError(t, runIssueMutation(ctx, gh, 42))
			if snapshot {
				require.NoError(t, h.repos[1].UpdateTask(ctx, stale))
			} else {
				_, err = h.services[1].UpdateTask(ctx, "issue-task", &taskservice.UpdateTaskRequest{Metadata: map[string]interface{}{"replacement": true}})
				require.NoError(t, err)
			}
			stored, err := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			assertIssueMutationIdentity(t, stored.Metadata, 0)
		})
	}
}
