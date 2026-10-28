package hostcaps

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestRegistryHoldsTheClosedNameSetAtVersionOne(t *testing.T) {
	want := []string{
		"workspace_descriptor", "repository_intent", "tunnel_transport", "api_url_forward",
		"drain_receipt", "destroy_checkpoints", "durable_bootstrap", "admission_context",
		"pending_placement", "provider_operations", "provider_maintenance", "launch_policy",
		"preparation_hints", "typed_profile_values", "secret_references", "provision_refusal",
	}
	got := Registry()
	if len(got) != len(want) {
		t.Fatalf("registry has %d entries, want %d", len(got), len(want))
	}
	for i, name := range want {
		entry := got[i]
		if entry.Name != name || entry.Min != 1 || entry.Max != 1 || entry.IntroducedIn == "" {
			t.Fatalf("entry %d = %+v, want %s at version 1 with an introduced_in release", i, entry, name)
		}
	}
}

func TestRegistryReturnsACopy(t *testing.T) {
	first := Registry()
	first[0].Name = "mutated"
	first[1].Requires = append(first[1].Requires, "mutated")
	if Registry()[0].Name == "mutated" {
		t.Fatal("mutating the returned registry changed the host registry")
	}
}

func TestValidateRequirements(t *testing.T) {
	cases := []struct {
		name string
		reqs []Requirement
		want string
	}{
		{"empty list is valid", nil, ""},
		{"known capability", []Requirement{{"workspace_descriptor", 1}}, ""},
		{"unknown name", []Requirement{{"teleport", 1}}, `unknown capability "teleport"`},
		{"duplicate name", []Requirement{{"workspace_descriptor", 1}, {"workspace_descriptor", 1}}, `duplicate capability "workspace_descriptor"`},
		{"zero version", []Requirement{{"workspace_descriptor", 0}}, "version must be an integer of at least 1"},
		{"negative version", []Requirement{{"workspace_descriptor", -3}}, "version must be an integer of at least 1"},
		{"version above range", []Requirement{{"workspace_descriptor", 2}}, `capability "workspace_descriptor" version 2 is outside the supported range 1-1`},
		{"missing dependency is named", []Requirement{{"provider_maintenance", 1}}, `capability "provider_maintenance" requires "provider_operations"`},
		{"pending placement needs three", []Requirement{{"pending_placement", 1}}, `capability "pending_placement" requires "destroy_checkpoints"`},
		{"provision refusal needs checkpoints", []Requirement{{"provision_refusal", 1}}, `capability "provision_refusal" requires "destroy_checkpoints"`},
		{"launch policy needs descriptor", []Requirement{{"launch_policy", 1}}, `capability "launch_policy" requires "workspace_descriptor"`},
		{"preparation hints need intent", []Requirement{{"preparation_hints", 1}}, `capability "preparation_hints" requires "repository_intent"`},
		{"dependency satisfied", []Requirement{{"provider_maintenance", 1}, {"provider_operations", 1}}, ""},
		{"typed values need nothing", []Requirement{{"typed_profile_values", 1}, {"secret_references", 1}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateRequirements(tc.reqs)
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			var joined []string
			for _, err := range errs {
				joined = append(joined, err.Error())
			}
			if !strings.Contains(strings.Join(joined, "\n"), tc.want) {
				t.Fatalf("errors %q do not contain %q", joined, tc.want)
			}
		})
	}
}

func TestValidateRejectsALongerListThanTheRegistry(t *testing.T) {
	var reqs []Requirement
	for range len(Registry()) + 1 {
		reqs = append(reqs, Requirement{"workspace_descriptor", 1})
	}
	errs := ValidateRequirements(reqs)
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "at most") {
		t.Fatalf("expected a list-length error first, got %v", errs)
	}
}

func TestValidateReportsPairsRetiredByThisHost(t *testing.T) {
	restore := setRetiredForTest([]RetiredPair{{Name: "old_thing", Version: 1, RetiredIn: "9.9.9"}})
	defer restore()
	errs := ValidateRequirements([]Requirement{{"old_thing", 1}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "capability_retired") || !strings.Contains(errs[0].Error(), "9.9.9") {
		t.Fatalf("expected capability_retired naming the release, got %v", errs)
	}
}

func TestHighestIntroducedIn(t *testing.T) {
	if got := HighestIntroducedIn(nil); got != "" {
		t.Fatalf("no capability must give no minimum, got %q", got)
	}
	got := HighestIntroducedIn([]Requirement{{"workspace_descriptor", 1}, {"secret_references", 1}})
	if got == "" {
		t.Fatal("expected a release for declared capabilities")
	}
}

func TestDeprecationWindowIsAtLeastTwoStableReleases(t *testing.T) {
	restore := setRegistryForTest([]Entry{{Name: "a", Min: 1, Max: 1, IntroducedIn: "1.0.0", DeprecatedIn: "1.2.0", RetireIn: "1.3.0"}})
	defer restore()
	if err := CheckDeprecationWindows(); err == nil {
		t.Fatal("retire_in one release after deprecated_in must fail the window check")
	}
	restore2 := setRegistryForTest([]Entry{{Name: "a", Min: 1, Max: 1, IntroducedIn: "1.0.0", DeprecatedIn: "1.2.0", RetireIn: "1.4.0"}})
	defer restore2()
	if err := CheckDeprecationWindows(); err != nil {
		t.Fatalf("two releases must pass: %v", err)
	}
}

func TestDeprecatedVersionProducesAWarningNamingTheReplacement(t *testing.T) {
	restore := setRegistryForTest([]Entry{{Name: "a", Min: 1, Max: 2, IntroducedIn: "1.0.0", DeprecatedIn: "1.2.0", RetireIn: "1.5.0", Replacement: "a@2"}})
	defer restore()
	warnings := DeprecationWarnings([]Requirement{{"a", 1}})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "a@2") {
		t.Fatalf("expected a warning naming the replacement, got %v", warnings)
	}
}

// The retired list is append-only: every pair recorded in the committed golden
// file from the previous release must still be present, and no live registry
// version may reuse a retired pair.
func TestRetiredListIsAppendOnly(t *testing.T) {
	file, err := os.Open("testdata/retired.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	have := map[string]bool{}
	for _, pair := range Retired() {
		have[pair.Key()] = true
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !have[line] {
			t.Fatalf("retired pair %q was removed from the append-only list", line)
		}
	}
	for _, entry := range Registry() {
		for version := entry.Min; version <= entry.Max; version++ {
			if have[RetiredPair{Name: entry.Name, Version: version}.Key()] {
				t.Fatalf("live capability %s@%d reuses a retired pair", entry.Name, version)
			}
		}
	}
}
