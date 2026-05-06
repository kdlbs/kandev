package service_test

import (
	"context"
	"testing"
)

func TestLogActivity_AndList(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.LogActivity(ctx, "ws-1", "user", "user-1",
		"agent.created", "agent", "agent-1", `{"name":"test"}`)
	svc.LogActivity(ctx, "ws-1", "system", "budget_checker",
		"budget.alert", "agent", "agent-1", `{}`)

	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("count = %d, want 2", len(entries))
	}
}

func TestListActivityFiltered_ByType(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.LogActivity(ctx, "ws-1", "user", "user-1",
		"agent.created", "agent", "agent-1", `{}`)
	svc.LogActivity(ctx, "ws-1", "system", "budget_checker",
		"budget.alert", "agent", "agent-2", `{}`)
	svc.LogActivity(ctx, "ws-1", "user", "user-1",
		"project.created", "project", "proj-1", `{}`)

	// Filter by "agent" type.
	entries, err := svc.ListActivityFiltered(ctx, "ws-1", "agent", 50)
	if err != nil {
		t.Fatalf("ListActivityFiltered: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("count = %d, want 2 (agent-related entries)", len(entries))
	}

	// Filter by "all" returns everything.
	all, err := svc.ListActivityFiltered(ctx, "ws-1", "all", 50)
	if err != nil {
		t.Fatalf("ListActivityFiltered all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("count = %d, want 3", len(all))
	}
}

func TestListRecentActivity(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 15; i++ {
		svc.LogActivity(ctx, "ws-1", "user", "user-1",
			"test.action", "test", "target-1", `{}`)
	}

	entries, err := svc.ListRecentActivity(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListRecentActivity: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("count = %d, want 10", len(entries))
	}
}
