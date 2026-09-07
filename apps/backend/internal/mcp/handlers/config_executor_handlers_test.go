package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingExecutorRepository struct {
	repository.ExecutorRepository
	firstUpdateStarted chan struct{}
	releaseFirstUpdate chan struct{}
	gateMu             sync.Mutex
	firstUpdate        bool
}

func (r *blockingExecutorRepository) blockFirstUpdate() {
	r.gateMu.Lock()
	if r.firstUpdate {
		r.gateMu.Unlock()
		return
	}
	r.firstUpdate = true
	r.gateMu.Unlock()
	close(r.firstUpdateStarted)
	<-r.releaseFirstUpdate
}

func (r *blockingExecutorRepository) UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	r.blockFirstUpdate()
	return r.ExecutorRepository.UpdateExecutorProfile(ctx, profile)
}

func (r *blockingExecutorRepository) UpdateExecutorProfileIfUnmodified(
	ctx context.Context,
	profile *models.ExecutorProfile,
	expectedUpdatedAt time.Time,
) error {
	r.blockFirstUpdate()
	return r.ExecutorRepository.UpdateExecutorProfileIfUnmodified(ctx, profile, expectedUpdatedAt)
}

func TestRejectOperatorConfigKeys_RejectsReservedKey(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{
		allowUserNamespacesProfileConfigKey: "true",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), allowUserNamespacesProfileConfigKey)
}

func TestRejectOperatorConfigKeys_AcceptsNormalKeys(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{
		"image_tag": "kandev/agent:latest",
	})
	assert.NoError(t, err)
}

func TestRejectOperatorConfigKeys_AcceptsEmptyConfig(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{})
	assert.NoError(t, err)
}

func TestRejectOperatorConfigKeys_AcceptsNilConfig(t *testing.T) {
	err := rejectOperatorConfigKeys(nil)
	assert.NoError(t, err)
}

func TestExecutorProfileHandlersRejectUserNamespacesAndPreserveOperatorConfig(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTaskService(t)
	executor, err := svc.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name: "Docker", Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive, Resumable: true,
	})
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	create := makeWSMessage(t, ws.ActionMCPCreateExecutorProfile, map[string]any{
		"executor_id": executor.ID, "name": "Blocked", "config": map[string]string{
			allowUserNamespacesProfileConfigKey: "true", "image_tag": "kandev-agent:e2e",
		},
	})
	response, err := h.handleCreateExecutorProfile(ctx, create)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
	assert.Contains(t, string(response.Payload), allowUserNamespacesProfileConfigKey)
	profiles, err := svc.ListExecutorProfiles(ctx, executor.ID)
	require.NoError(t, err)
	assert.Empty(t, profiles)

	profile, err := svc.CreateExecutorProfile(ctx, &service.CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "Operator", Config: map[string]string{
			allowUserNamespacesProfileConfigKey: "true",
		},
	})
	require.NoError(t, err)
	update := makeWSMessage(t, ws.ActionMCPUpdateExecutorProfile, map[string]any{
		"profile_id": profile.ID, "config": map[string]string{allowUserNamespacesProfileConfigKey: "false"},
	})
	response, err = h.handleUpdateExecutorProfile(ctx, update)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
	stored, err := svc.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "true", stored.Config[allowUserNamespacesProfileConfigKey])

	name := "Renamed by MCP"
	update = makeWSMessage(t, ws.ActionMCPUpdateExecutorProfile, map[string]any{
		"profile_id": profile.ID, "name": name,
	})
	response, err = h.handleUpdateExecutorProfile(ctx, update)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	stored, err = svc.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, name, stored.Name)
	assert.Equal(t, "true", stored.Config[allowUserNamespacesProfileConfigKey])
}

func TestHandleCreateExecutorProfileAcceptsNormalConfig(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTaskService(t)
	executor, err := svc.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name: "Docker", Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive, Resumable: true,
	})
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	response, err := h.handleCreateExecutorProfile(ctx, makeWSMessage(t, ws.ActionMCPCreateExecutorProfile, map[string]any{
		"executor_id": executor.ID, "name": "Allowed", "config": map[string]string{"image_tag": "kandev-agent:e2e"},
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	profiles, err := svc.ListExecutorProfiles(ctx, executor.ID)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "kandev-agent:e2e", profiles[0].Config["image_tag"])
}

func TestHandleUpdateExecutorProfilePreservesOperatorConfigDuringOrdinaryConfigUpdate(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestTaskService(t)
	executor, err := svc.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name: "Docker", Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive, Resumable: true,
	})
	require.NoError(t, err)
	profile, err := svc.CreateExecutorProfile(ctx, &service.CreateExecutorProfileRequest{
		ExecutorID: executor.ID,
		Name:       "Operator",
		Config: map[string]string{
			allowUserNamespacesProfileConfigKey: "true",
			"image_tag":                         "kandev-agent:old",
		},
	})
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	response, err := h.handleUpdateExecutorProfile(ctx, makeWSMessage(
		t, ws.ActionMCPUpdateExecutorProfile, map[string]any{
			"profile_id": profile.ID,
			"config":     map[string]string{"image_tag": "kandev-agent:new"},
		},
	))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	stored, err := svc.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "true", stored.Config[allowUserNamespacesProfileConfigKey])
	assert.Equal(t, "kandev-agent:new", stored.Config["image_tag"])
}

func TestHandleUpdateExecutorProfileUsesCASWhenConfigIsOmitted(t *testing.T) {
	ctx := context.Background()
	_, repo, eventBus := newTestTaskServiceWithEventBus(t)
	blockingRepo := &blockingExecutorRepository{
		ExecutorRepository: repo,
		firstUpdateStarted: make(chan struct{}),
		releaseFirstUpdate: make(chan struct{}),
	}
	svc := service.NewService(service.Repos{Executors: blockingRepo}, eventBus, testLogger(t).WithFields(), service.RepositoryDiscoveryConfig{})
	executor, err := svc.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name: "Docker", Type: models.ExecutorTypeLocalDocker, Status: models.ExecutorStatusActive, Resumable: true,
	})
	require.NoError(t, err)
	profile, err := svc.CreateExecutorProfile(ctx, &service.CreateExecutorProfileRequest{
		ExecutorID: executor.ID,
		Name:       "Operator",
		Config:     map[string]string{},
	})
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	updateMessage := makeWSMessage(t, ws.ActionMCPUpdateExecutorProfile, map[string]any{
		"profile_id": profile.ID,
		"name":       "MCP name",
	})

	resultCh := make(chan struct {
		response *ws.Message
		err      error
	}, 1)
	go func() {
		response, updateErr := h.handleUpdateExecutorProfile(ctx, updateMessage)
		resultCh <- struct {
			response *ws.Message
			err      error
		}{response: response, err: updateErr}
	}()

	select {
	case <-blockingRepo.firstUpdateStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP update did not reach the write barrier")
	}
	operatorEnabled := map[string]string{allowUserNamespacesProfileConfigKey: "true"}
	_, err = svc.UpdateExecutorProfile(ctx, profile.ID, &service.UpdateExecutorProfileRequest{Config: operatorEnabled})
	require.NoError(t, err)
	close(blockingRepo.releaseFirstUpdate)

	var result struct {
		response *ws.Message
		err      error
	}
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP update did not complete after the write barrier was released")
	}
	require.NoError(t, result.err)
	assertWSError(t, result.response, ws.ErrorCodeInternalError)
	stored, err := svc.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "true", stored.Config[allowUserNamespacesProfileConfigKey])
}
