package mergestate

import (
	"sort"
	"testing"
)

func TestChangeOrderingByProviderRepoNumber(t *testing.T) {
	keys := []ChangeKey{
		{Provider: "gitlab", Host: "gitlab.com", RepoID: "1", Number: 2},
		{Provider: "github", Host: "github.com", RepoID: "9", Number: 10},
		{Provider: "github", Host: "github.com", RepoID: "9", Number: 2},
		{Provider: "github", Host: "github.com", RepoID: "10", Number: 1},
		{Provider: "github", Host: "GitHub.com", RepoID: "9", Number: 1},
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	want := []string{
		"github|github.com|9|1",
		"github|github.com|9|2",
		"github|github.com|9|10",
		"github|github.com|10|1",
		"gitlab|gitlab.com|1|2",
	}
	for i, k := range keys {
		if k.String() != want[i] {
			t.Fatalf("position %d: got %q want %q", i, k.String(), want[i])
		}
	}
}

func TestChangeKeyHostNormalized(t *testing.T) {
	a := ChangeKey{Provider: "github", Host: "GitHub.com.", RepoID: "1", Number: 1}
	b := ChangeKey{Provider: "github", Host: "github.com", RepoID: "1", Number: 1}
	if a.String() != b.String() {
		t.Fatalf("%q != %q", a.String(), b.String())
	}
}

func TestParseChangeKeyRoundTrip(t *testing.T) {
	k := ChangeKey{Provider: "gitlab", Host: "GitLab.Example.com.", RepoID: "r9", Number: 12}
	got, ok := ParseChangeKey(k.String())
	if !ok || got.String() != k.String() {
		t.Fatalf("round trip = %+v %v", got, ok)
	}
	for _, bad := range []string{"", "a|b|c", "github|h|r|0", "github|h||3", "github|h|r|x", "a|b|c|d|1"} {
		if _, ok := ParseChangeKey(bad); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
}
