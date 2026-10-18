package lsp

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeControllerTasks struct {
	mu             sync.Mutex
	authErr        error
	environmentErr error
	admissionErr   error
	admissionLock  *sync.RWMutex
	calls          []string
	environments   map[string]*models.TaskEnvironment
	task           *models.Task
}

func (f *fakeControllerTasks) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	f.record("authorize:" + taskID)
	return f.authErr
}

func (f *fakeControllerTasks) AcquireTaskLSPAdmission(_ context.Context, taskID string) (func(), error) {
	f.record("admission:" + taskID)
	if f.admissionErr != nil {
		return nil, f.admissionErr
	}
	if f.admissionLock != nil {
		if !f.admissionLock.TryRLock() {
			return nil, errors.New("task LSP admission blocked")
		}
		return f.admissionLock.RUnlock, nil
	}
	return func() {}, nil
}

func (f *fakeControllerTasks) GetTask(_ context.Context, taskID string) (*models.Task, error) {
	f.record("task:" + taskID)
	if f.task != nil {
		return f.task, nil
	}
	return &models.Task{ID: taskID}, nil
}

func (f *fakeControllerTasks) GetTaskEnvironmentForTaskLSP(_ context.Context, taskID string) (*models.TaskEnvironment, error) {
	f.record("environment:" + taskID)
	if f.environmentErr != nil {
		return nil, f.environmentErr
	}
	if f.environments != nil {
		return f.environments[taskID], nil
	}
	return readyEnvironment(taskID, executorTypeLocalPC), nil
}

func (f *fakeControllerTasks) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeControllerTasks) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeControllerTasks) resetCalls() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}

func (f *fakeLSPHost) DialTaskLSPAttach(
	context.Context, string, uint64,
) (*websocket.Conn, *http.Response, error) {
	return nil, nil, errors.New("not dialed by controller tests")
}
