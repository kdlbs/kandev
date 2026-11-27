package executor

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type interruptedGenerationRepository struct {
	*mockRepository
	generation *models.HarnessSessionGeneration
}

func (r *interruptedGenerationRepository) GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error) {
	return r.generation, nil
}

func TestInterruptedNativeResumeLaunchContract(t *testing.T) {
	options := ResumeOptions{RequiredNativeConversationID: "native", NoInitialPrompt: true}
	field := reflect.ValueOf(&options).Elem().FieldByName("InterruptedSubmissionID")
	require.True(t, field.IsValid(), "native continuation must name only the interrupted submission")
	field.SetString("old-prompt")
	reflect.ValueOf(&options).Elem().FieldByName("InterruptedHarnessGeneration").SetUint(3)
	reflect.ValueOf(&options).Elem().FieldByName("InterruptedStreamID").SetString("old-stream")
	req, _ := newResumeLaunchRequest(&v1.Task{ID: "task", Description: "must not resend"}, &models.TaskSession{ID: "session", QueueIncarnationID: "incarnation"}, true, options)
	require.Empty(t, req.TaskDescription)
	require.Equal(t, "native", req.RequiredNativeConversationID)
	require.Equal(t, "old-prompt", reflect.ValueOf(req).Elem().FieldByName("InterruptedSubmissionID").String())
	repo := &interruptedGenerationRepository{mockRepository: newMockRepository(), generation: &models.HarnessSessionGeneration{Generation: 3, NativeSessionID: "native"}}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	require.NoError(t, exec.applyDeliveryIdentity(context.Background(), req, &models.TaskSession{ID: "session", QueueIncarnationID: "incarnation"}))
	require.EqualValues(t, 4, req.DeliveryHarnessGeneration)
	require.False(t, req.ForceContextContinuation)
	repo.generation.NativeSessionID = "different-native"
	require.Error(t, exec.applyDeliveryIdentity(context.Background(), req, &models.TaskSession{ID: "session", QueueIncarnationID: "incarnation"}))
}
