package dashboard_test

import (
	"context"
	"testing"
)

// Compatibility control: this surface deliberately permits deeper cycles.
func TestTaskHierarchyAdmissionOfficePolicy(t *testing.T) {
	deps := newTestDeps(t)
	for _, id := range []string{"a", "b", "c"} {
		insertTestTask(t, deps.db, id, "workspace", id, "todo", 1)
	}
	ctx := context.Background()
	for _, edge := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}} {
		if err := deps.svc.UpdateTaskParentID(ctx, edge[0], edge[1]); err != nil {
			t.Fatalf("deliberate Office edge %v rejected: %v", edge, err)
		}
	}
	if err := deps.svc.UpdateTaskParentID(ctx, "a", "a"); err == nil {
		t.Fatal("direct self-reference accepted")
	}
	if err := deps.svc.UpdateTaskParentID(ctx, "a", "missing"); err == nil {
		t.Fatal("missing parent accepted")
	}
	var parent string
	if err := deps.db.Get(&parent, `SELECT parent_id FROM tasks WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	if parent != "b" {
		t.Fatalf("loser changed parent to %q", parent)
	}
}
