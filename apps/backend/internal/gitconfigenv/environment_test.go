package gitconfigenv

import "testing"

func TestIndexedGitConfigMergePreservesOrderedEntries(t *testing.T) {
	merged, err := Merge(map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "core.hooksPath",
		"GIT_CONFIG_VALUE_0": "/opt/locstat/hooks",
		"GIT_CONFIG_KEY_1":   "notes.augment.mergeStrategy",
		"GIT_CONFIG_VALUE_1": "cat_sort_uniq",
	}, map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0": "!agentctl git-credential",
		"GIT_CONFIG_KEY_1":   "credential.useHttpPath",
		"GIT_CONFIG_VALUE_1": "true",
	})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	if got := merged["GIT_CONFIG_COUNT"]; got != "4" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 4", got)
	}
	if got := merged["GIT_CONFIG_KEY_1"]; got != "notes.augment.mergeStrategy" {
		t.Fatalf("GIT_CONFIG_KEY_1 = %q, want host entry", got)
	}
	if got := merged["GIT_CONFIG_KEY_2"]; got != "credential.https://github.com.helper" {
		t.Fatalf("GIT_CONFIG_KEY_2 = %q, want managed entry", got)
	}
}

func TestIndexedGitConfigMergeCollapsesOnlyExactBoundaryOverlap(t *testing.T) {
	managedHelper := Entry{Key: "credential.https://github.com.helper", Value: "!agentctl git-credential"}
	merged, err := Merge(environmentWithEntries(
		Entry{Key: "safe.directory", Value: "*"},
		managedHelper,
	), environmentWithEntries(
		managedHelper,
		Entry{Key: "credential.useHttpPath", Value: "true"},
	))
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	if got := merged["GIT_CONFIG_COUNT"]; got != "3" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 3", got)
	}
	if got := merged["GIT_CONFIG_KEY_2"]; got != "credential.useHttpPath" {
		t.Fatalf("GIT_CONFIG_KEY_2 = %q, want trailing overlay entry", got)
	}
}

func TestIndexedGitConfigMergeRetainsMeaningfulRepeatedEntries(t *testing.T) {
	merged, err := Merge(environmentWithEntries(
		Entry{Key: "url.https://github.com/.insteadOf", Value: "git@github.com:"},
	), environmentWithEntries(
		Entry{Key: "url.https://github.com/.insteadOf", Value: "ssh://git@github.com/"},
	))
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	if got := merged["GIT_CONFIG_COUNT"]; got != "2" {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want 2", got)
	}
}

func TestIndexedGitConfigMergeRejectsMalformedBlock(t *testing.T) {
	_, err := Merge(map[string]string{
		"GIT_CONFIG_COUNT": "2",
		"GIT_CONFIG_KEY_0": "core.hooksPath",
	}, nil)
	if err == nil {
		t.Fatal("Merge() error = nil, want malformed Git config error")
	}
}

func TestFilterRemovesOnlyRequestedIndexedEntries(t *testing.T) {
	env := map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "core.hooksPath",
		"GIT_CONFIG_VALUE_0": "/work/hooks",
		"GIT_CONFIG_KEY_1":   "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1": "!agentctl git-credential",
		"UNCHANGED":          "yes",
	}

	filtered, err := Filter(env, func(index int, entries []Entry) bool {
		return entries[index].Value != "!agentctl git-credential"
	})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if got, want := filtered["GIT_CONFIG_COUNT"], "1"; got != want {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want %q", got, want)
	}
	if got, want := filtered["GIT_CONFIG_KEY_0"], "core.hooksPath"; got != want {
		t.Fatalf("GIT_CONFIG_KEY_0 = %q, want %q", got, want)
	}
	if got, want := filtered["UNCHANGED"], "yes"; got != want {
		t.Fatalf("UNCHANGED = %q, want %q", got, want)
	}
}

func environmentWithEntries(entries ...Entry) map[string]string {
	env := map[string]string{}
	writeEntries(env, entries)
	return env
}
