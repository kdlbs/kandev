package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type barrierCreateTaskRepo struct {
	*acFakeTaskRepo
	arrived chan struct{}
	release chan struct{}
}

func (r *barrierCreateTaskRepo) CreateTask(ctx context.Context, task *models.Task) error {
	r.arrived <- struct{}{}
	<-r.release
	return r.acFakeTaskRepo.CreateTask(ctx, task)
}

func TestEnsureUsesOneConversationAcrossServiceInstances(t *testing.T) {
	tasks := &barrierCreateTaskRepo{
		acFakeTaskRepo: &acFakeTaskRepo{},
		arrived:        make(chan struct{}, 2),
		release:        make(chan struct{}),
	}
	sessions := newACFakeSessionRepo()
	profiles := newACFakeProfileRepo()
	state := newACFakeStateRepo()
	first := NewAgentConversationService(tasks, sessions, profiles, state, &acFakeEventBus{})
	second := NewAgentConversationService(tasks, sessions, profiles, state, &acFakeEventBus{})
	first.SetDispatcher(newACFakeDispatcher())
	second.SetDispatcher(newACFakeDispatcher())

	type result struct {
		desc pluginsdk.AgentConversationDescriptor
		err  error
	}
	results := make(chan result, 2)
	ensure := func(svc *AgentConversationService) {
		desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"})
		results <- result{desc: desc, err: err}
	}
	go ensure(first)
	go ensure(second)
	<-tasks.arrived
	<-tasks.arrived
	close(tasks.release)

	firstResult := <-results
	secondResult := <-results
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("concurrent Ensure errors = %v, %v", firstResult.err, secondResult.err)
	}
	if firstResult.desc.TaskID != secondResult.desc.TaskID {
		t.Fatalf("task IDs = %q, %q; two service instances must converge on one backing task", firstResult.desc.TaskID, secondResult.desc.TaskID)
	}
	if got := tasks.count(); got != 1 {
		t.Fatalf("task count = %d, want one shared backing task", got)
	}
	if got := sessions.count(); got != 1 {
		t.Fatalf("primary session count = %d, want one shared primary session", got)
	}
}
