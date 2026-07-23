package gitlab

import (
	"context"
	"reflect"
	"testing"
)

func TestStoreMentionScope_RoundTripsAndIsolatesWorkspaces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	want := &MentionScope{
		WorkspaceID: "workspace-1",
		Host:        "https://gitlab.example.test/base",
		Projects: []MentionProjectScope{
			{ID: 101, Path: "group/api"},
			{ID: 202, Path: "group/web"},
		},
	}
	if err := store.UpsertMentionScope(ctx, want); err != nil {
		t.Fatalf("upsert mention scope: %v", err)
	}

	got, err := store.GetMentionScope(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("get mention scope: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scope = %#v, want %#v", got, want)
	}
	other, err := store.GetMentionScope(ctx, "workspace-2")
	if err != nil {
		t.Fatalf("get other workspace scope: %v", err)
	}
	if other != nil {
		t.Fatalf("other workspace scope = %#v, want nil", other)
	}

	want.Projects = []MentionProjectScope{{ID: 303, Path: "other/mobile"}}
	if err := store.UpsertMentionScope(ctx, want); err != nil {
		t.Fatalf("update mention scope: %v", err)
	}
	updated, err := store.GetMentionScope(ctx, "workspace-1")
	if err != nil {
		t.Fatalf("get updated scope: %v", err)
	}
	if !reflect.DeepEqual(updated, want) {
		t.Fatalf("updated scope = %#v, want %#v", updated, want)
	}
}
