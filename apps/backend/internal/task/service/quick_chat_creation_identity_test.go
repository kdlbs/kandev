package service

import (
	"context"
	"sync"
	"testing"
)

func TestQuickChatCreationIdentityBindsCanonicalTokenToPrincipal(t *testing.T) {
	creationID := "a7c0fd51-6526-4732-95bc-c6e25f745e7f"
	externalID, err := NewQuickChatCreationExternalID("user-a", creationID)
	if err != nil {
		t.Fatalf("NewQuickChatCreationExternalID: %v", err)
	}
	if got, ok := QuickChatCreationIDForPrincipal("user-a", externalID); !ok || got != creationID {
		t.Fatalf("QuickChatCreationIDForPrincipal(user-a) = %q, %v; want %q, true", got, ok, creationID)
	}
	if _, ok := QuickChatCreationIDForPrincipal("user-b", externalID); ok {
		t.Fatal("a different principal must not recover the creation token")
	}
	if _, err := NewQuickChatCreationExternalID("user-a", "A7C0FD51-6526-4732-95BC-C6E25F745E7F"); err == nil {
		t.Fatal("a noncanonical UUID must be rejected")
	}
}

func TestQuickChatCreationIdentitySQLiteUniqueness(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-quick-chat-race")
	externalID, err := NewQuickChatCreationExternalID("user-a", "c30965fd-95ee-4322-8fbb-780eb3151bc1")
	if err != nil {
		t.Fatalf("NewQuickChatCreationExternalID: %v", err)
	}
	start := make(chan struct{})
	results := make([]CreateTaskResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-quick-chat-race", WorkflowID: wfID, Title: "Same creation",
				ExternalID: externalID, AllowReservedExternalID: true,
			})
		}()
	}
	close(start)
	wg.Wait()

	createdCount, foundCount := 0, 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		switch results[i].Outcome {
		case CreateTaskOutcomeCreated:
			createdCount++
		case CreateTaskOutcomeFoundSettled, CreateTaskOutcomeFoundUnsettled:
			foundCount++
		}
	}
	if createdCount != 1 || foundCount != 1 {
		t.Fatalf("outcomes = %#v, want one Created and one Found", results)
	}
	if results[0].Task.ID != results[1].Task.ID {
		t.Fatalf("same token returned different tasks: %s and %s", results[0].Task.ID, results[1].Task.ID)
	}
	if count := countTasksHoldingExternalID(t, ctx, repo, "ws-quick-chat-race", externalID); count != 1 {
		t.Fatalf("tasks holding creation identity = %d, want 1", count)
	}
}
