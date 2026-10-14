package watch

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeReader struct {
	sets     []SetMembers
	repos    []string
	setsErr  error
	reposErr error
	setCalls int
	repCalls int
}

func (f *fakeReader) Sets(context.Context, string) ([]SetMembers, error) {
	f.setCalls++
	return f.sets, f.setsErr
}

func (f *fakeReader) RepositoryIDs(context.Context, string) ([]string, error) {
	f.repCalls++
	return f.repos, f.reposErr
}

func resolve(t *testing.T, r *fakeReader, s Scope) Resolved {
	t.Helper()
	got, err := NewResolver(r).Resolve(context.Background(), "ws", s)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return got
}

func TestInProjectsMatrix(t *testing.T) {
	r := &fakeReader{
		sets:  []SetMembers{{ID: "s1", RepositoryIDs: []string{"a", "b"}}, {ID: "s2", RepositoryIDs: []string{"c"}}},
		repos: []string{"a", "b", "c", "d"},
	}
	cases := []struct {
		name  string
		scope Scope
		task  []string
		want  bool
	}{
		{"all matches anything", Scope{}, []string{"zzz"}, true},
		{"all matches no repository", Scope{}, nil, true},
		{"listed repository", Scope{Selected: true, RepoIDs: []string{"d"}}, []string{"d"}, true},
		{"unlisted repository", Scope{Selected: true, RepoIDs: []string{"d"}}, []string{"a"}, false},
		{"repository in a listed set", Scope{Selected: true, SetIDs: []string{"s1"}}, []string{"b"}, true},
		{"repository in an unlisted set", Scope{Selected: true, SetIDs: []string{"s1"}}, []string{"c"}, false},
		{"no repository, toggle on", Scope{Selected: true, SetIDs: []string{"s1"}, IncludeNoRepo: true}, nil, true},
		{"no repository, toggle off", Scope{Selected: true, SetIDs: []string{"s1"}}, nil, false},
		{"empty slice is no repository", Scope{Selected: true, IncludeNoRepo: true}, []string{}, true},
		{"multi repository any match", Scope{Selected: true, RepoIDs: []string{"d"}}, []string{"a", "d"}, true},
		{"multi repository none match", Scope{Selected: true, RepoIDs: []string{"d"}}, []string{"a", "b"}, false},
		{"selected and empty watches nothing", Scope{Selected: true}, []string{"a"}, false},
		{"selected empty with toggle watches only repository-less", Scope{Selected: true, IncludeNoRepo: true}, []string{"a"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolve(t, r, tc.scope).InProjects(tc.task); got != tc.want {
				t.Fatalf("InProjects(%v) = %v, want %v", tc.task, got, tc.want)
			}
		})
	}
}

func TestRepositoryEntryIntersectedWithLiveRepositories(t *testing.T) {
	r := &fakeReader{repos: []string{"a"}}
	got := resolve(t, r, Scope{Selected: true, RepoIDs: []string{"a", "gone"}})
	if got.InProjects([]string{"gone"}) {
		t.Fatal("a deleted repository must not keep matching")
	}
	if !got.InProjects([]string{"a"}) {
		t.Fatal("a live listed repository must match")
	}
	if want := []string{"a"}; !reflect.DeepEqual(got.RepositoryIDs(), want) {
		t.Fatalf("RepositoryIDs = %v, want %v", got.RepositoryIDs(), want)
	}
}

func TestRepositoryAddedToSetWidensOnNextResolve(t *testing.T) {
	r := &fakeReader{sets: []SetMembers{{ID: "s1", RepositoryIDs: []string{"a"}}}, repos: []string{"a", "b"}}
	scope := Scope{Selected: true, SetIDs: []string{"s1"}}
	if resolve(t, r, scope).InProjects([]string{"b"}) {
		t.Fatal("b is not a member yet")
	}
	r.sets = []SetMembers{{ID: "s1", RepositoryIDs: []string{"a", "b"}}}
	if !resolve(t, r, scope).InProjects([]string{"b"}) {
		t.Fatal("membership is resolved live, so b matches now")
	}
	r.sets = []SetMembers{{ID: "s1"}}
	if resolve(t, r, scope).InProjects([]string{"a"}) {
		t.Fatal("a set that lost its last repository matches nothing")
	}
}

func TestResolveSkipsReadsUnderAll(t *testing.T) {
	r := &fakeReader{setsErr: errors.New("boom"), reposErr: errors.New("boom")}
	got, err := NewResolver(r).Resolve(context.Background(), "ws", Scope{})
	if err != nil {
		t.Fatalf("scope all must not read: %v", err)
	}
	if r.setCalls+r.repCalls != 0 || !got.InProjects([]string{"x"}) {
		t.Fatalf("calls=%d/%d", r.setCalls, r.repCalls)
	}
}

func TestResolveReadsOnlyWhatItNeeds(t *testing.T) {
	r := &fakeReader{}
	resolve(t, r, Scope{Selected: true, IncludeNoRepo: true})
	if r.setCalls+r.repCalls != 0 {
		t.Fatalf("an entry-less selected scope needs no read, got %d/%d", r.setCalls, r.repCalls)
	}
}

func TestResolveFailureIsOneError(t *testing.T) {
	for _, r := range []*fakeReader{{setsErr: errors.New("sets")}, {reposErr: errors.New("repos")}} {
		_, err := NewResolver(r).Resolve(context.Background(), "ws", Scope{Selected: true, SetIDs: []string{"s"}, RepoIDs: []string{"r"}})
		if err == nil {
			t.Fatal("a failed read must fail the resolve")
		}
	}
}

func TestTaskCombinesWorkflowAndProjects(t *testing.T) {
	r := &fakeReader{repos: []string{"a"}}
	in := resolve(t, r, Scope{Selected: true, RepoIDs: []string{"a"}})
	if !Task(true, in, []string{"a"}) {
		t.Fatal("watched workflow and in-scope repository")
	}
	if Task(false, in, []string{"a"}) {
		t.Fatal("unwatched workflow")
	}
	if Task(true, in, []string{"b"}) {
		t.Fatal("out-of-scope repository")
	}
}

func TestEntriesOutsideScope(t *testing.T) {
	r := &fakeReader{sets: []SetMembers{{ID: "s1", RepositoryIDs: []string{"a"}}}, repos: []string{"a", "b"}}
	scope := Scope{Selected: true, SetIDs: []string{"s1"}, RepoIDs: []string{"b"}}
	if !resolve(t, r, scope).RepositoryInScope("b") || !resolve(t, r, scope).RepositoryInScope("a") {
		t.Fatal("repository in scope through entry or set")
	}
	if resolve(t, r, scope).RepositoryInScope("zzz") {
		t.Fatal("unknown repository is out")
	}
}
