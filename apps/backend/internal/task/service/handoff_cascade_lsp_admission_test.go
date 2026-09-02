package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/repository"
)

type blockingLSPMutationCleaner struct {
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	held    map[string]int
}

func (c *blockingLSPMutationCleaner) CleanupTaskResources(context.Context, string, bool) {}

func (c *blockingLSPMutationCleaner) CleanupTaskLSP(context.Context, string, string) error {
	close(c.entered)
	<-c.release
	return nil
}

func (c *blockingLSPMutationCleaner) acquireTaskLSPMutation(taskID string) func() {
	c.mu.Lock()
	if c.held == nil {
		c.held = make(map[string]int)
	}
	c.held[taskID]++
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			c.held[taskID]--
			c.mu.Unlock()
		})
	}
}

func (c *blockingLSPMutationCleaner) mutationHeld(taskID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.held[taskID] > 0
}

func TestTerminalTaskTreeMutationHoldsLSPAdmissionGuard(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *HandoffService) error
	}{
		{name: "archive", run: func(ctx context.Context, svc *HandoffService) error {
			_, err := svc.ArchiveTaskTree(ctx, "root", false)
			return err
		}},
		{name: "delete", run: func(ctx context.Context, svc *HandoffService) error {
			_, err := svc.DeleteTaskTree(ctx, "root", false)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tasks := newFakeTaskRepo()
			tasks.addTask("root", "", "ws-1")
			cleaner := &blockingLSPMutationCleaner{
				entered: make(chan struct{}), release: make(chan struct{}),
			}
			var taskRepo repository.TaskRepository = newCascadeRepo(tasks)
			if test.name == "delete" {
				taskRepo = &fakeDeleteRepo{fakeCascadeRepo: newCascadeRepo(tasks)}
			}
			svc := NewHandoffService(taskRepo, nil, nil, nil, newCascadeWSGroupRepo(), nil)
			svc.SetTaskResourceCleaner(cleaner)
			done := make(chan error, 1)
			go func() { done <- test.run(context.Background(), svc) }()
			<-cleaner.entered
			if !cleaner.mutationHeld("root") {
				t.Fatal("task LSP mutation guard was not held during cleanup")
			}
			close(cleaner.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if cleaner.mutationHeld("root") {
				t.Fatal("task LSP mutation guard remained held after mutation")
			}
		})
	}
}
