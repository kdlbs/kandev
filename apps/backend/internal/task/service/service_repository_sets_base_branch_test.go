package service

import (
	"context"
	"errors"
	"testing"
)

func TestCreateRepositorySetPersistsMemberBasesAndPublishesThem(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	set, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID: "ws-1",
		Name:        "Branch set",
		Repositories: []RepositorySetMemberInput{
			{RepositoryID: "repo-web", BaseBranch: "develop"},
			{RepositoryID: "repo-gateway"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	if len(set.Items) != 2 || set.Items[0].BaseBranch != "develop" || set.Items[1].BaseBranch != "" {
		t.Fatalf("created items = %+v", set.Items)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Fatalf("published events = %#v, want one event", events)
	}
	data := events[0].Data.(map[string]interface{})
	items := data["repositories"].([]map[string]interface{})
	if items[0]["base_branch"] != "develop" || items[1]["base_branch"] != "" {
		t.Fatalf("event members = %#v", items)
	}
}

func TestRepositorySetRejectsUnsafeBaseWithoutWriting(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID: "ws-1",
		Name:        "Unsafe",
		Repositories: []RepositorySetMemberInput{
			{RepositoryID: "repo-web", BaseBranch: "--upload-pack=evil"},
		},
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("unsafe base error = %v, want ErrInvalidRepositorySet", err)
	}
	sets, err := svc.ListRepositorySets(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("rejected set wrote %d rows", len(sets))
	}
}

func TestRepositorySetRejectsMalformedBaseRefWithoutWriting(t *testing.T) {
	for _, baseBranch := range []string{"main/", "a//b"} {
		t.Run(baseBranch, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedSetWorkspace(t, svc, repo)

			_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
				WorkspaceID: "ws-1",
				Name:        "Malformed base",
				Repositories: []RepositorySetMemberInput{
					{RepositoryID: "repo-web", BaseBranch: baseBranch},
				},
			})
			if !errors.Is(err, ErrInvalidRepositorySet) {
				t.Fatalf("malformed base error = %v, want ErrInvalidRepositorySet", err)
			}
		})
	}
}

func TestRepositorySetUpdateReplacesBasesAtomically(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	set := createFullStackSet(t, svc)

	updated, err := svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		Repositories: &[]RepositorySetMemberInput{
			{RepositoryID: "repo-orders", BaseBranch: "release"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}
	if len(updated.Items) != 1 || updated.Items[0].BaseBranch != "release" {
		t.Fatalf("updated items = %+v", updated.Items)
	}

	bad := []RepositorySetMemberInput{{RepositoryID: "repo-web", BaseBranch: "bad branch"}}
	_, err = svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		Name:         stringPointer("Rejected"),
		Repositories: &bad,
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("unsafe update error = %v, want ErrInvalidRepositorySet", err)
	}
	loaded, err := svc.GetRepositorySet(context.Background(), set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if loaded.Name != "Full-stack" || len(loaded.Items) != 1 || loaded.Items[0].BaseBranch != "release" {
		t.Fatalf("rejected update changed set = %+v", loaded)
	}
}

func TestRepositorySetRejectsConflictingMemberInputs(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSetWorkspace(t, svc, repo)
	ids := []string{"repo-web"}
	members := []RepositorySetMemberInput{{RepositoryID: "repo-gateway"}}

	_, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Conflict",
		RepositoryIDs: ids,
		Repositories:  members,
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("conflicting create error = %v, want ErrInvalidRepositorySet", err)
	}

	_, err = svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID:   "ws-1",
		Name:          "Conflict empty members",
		RepositoryIDs: ids,
		Repositories:  []RepositorySetMemberInput{},
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("conflicting create with empty members error = %v, want ErrInvalidRepositorySet", err)
	}

	set := createFullStackSet(t, svc)
	_, err = svc.UpdateRepositorySet(context.Background(), set.ID, &UpdateRepositorySetRequest{
		RepositoryIDs: &ids,
		Repositories:  &members,
	})
	if !errors.Is(err, ErrInvalidRepositorySet) {
		t.Fatalf("conflicting update error = %v, want ErrInvalidRepositorySet", err)
	}
}
