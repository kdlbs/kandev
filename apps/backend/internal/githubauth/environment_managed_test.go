package githubauth

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/gitconfigenv"
)

func TestManagedGitConfigOwnership(t *testing.T) {
	helper := "credential.https://github.com.helper"
	reset := gitconfigenv.Entry{Key: helper}
	managed := gitconfigenv.Entry{Key: helper, Value: ManagedGitCredentialHelper}
	usePath := gitconfigenv.Entry{Key: "credential.useHttpPath", Value: "true"}
	for _, tc := range []struct {
		name          string
		entries, want []gitconfigenv.Entry
	}{
		{"managed alone", []gitconfigenv.Entry{managed}, nil},
		{"generated block", []gitconfigenv.Entry{reset, managed, usePath}, nil},
		{"user path setting", []gitconfigenv.Entry{usePath, reset, managed, usePath}, []gitconfigenv.Entry{usePath}},
		{"user helper", []gitconfigenv.Entry{{Key: helper, Value: "!custom-helper"}}, []gitconfigenv.Entry{{Key: helper, Value: "!custom-helper"}}},
		{"standalone reset", []gitconfigenv.Entry{reset}, []gitconfigenv.Entry{reset}},
		{"different subsection", []gitconfigenv.Entry{{Key: "credential.https://github.com/Owner/Repo.helper"}, {Key: "credential.https://github.com/owner/repo.helper", Value: ManagedGitCredentialHelper}}, []gitconfigenv.Entry{{Key: "credential.https://github.com/Owner/Repo.helper"}}},
		{"case insensitive variable", []gitconfigenv.Entry{{Key: "Credential.https://github.com.Helper"}, managed}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"GIT_CONFIG_COUNT": strconv.Itoa(len(tc.entries))}
			for i, e := range tc.entries {
				env["GIT_CONFIG_KEY_"+strconv.Itoa(i)] = e.Key
				env["GIT_CONFIG_VALUE_"+strconv.Itoa(i)] = e.Value
			}
			got, err := gitconfigenv.Filter(env, func(i int, entries []gitconfigenv.Entry) bool {
				return !IsManagedGitCredentialConfigEntry(i, entries, true)
			})
			if err != nil {
				t.Fatal(err)
			}
			var entries []gitconfigenv.Entry
			count, _ := strconv.Atoi(got["GIT_CONFIG_COUNT"])
			for i := 0; i < count; i++ {
				entries = append(entries, gitconfigenv.Entry{Key: got["GIT_CONFIG_KEY_"+strconv.Itoa(i)], Value: got["GIT_CONFIG_VALUE_"+strconv.Itoa(i)]})
			}
			if !reflect.DeepEqual(entries, tc.want) {
				t.Fatalf("remaining config = %#v, want %#v", entries, tc.want)
			}
		})
	}
}

func TestLegacyGitConfigOwnership(t *testing.T) {
	entries := []gitconfigenv.Entry{
		{Key: "credential.https://github.com.helper"},
		{Key: "credential.https://github.com.helper", Value: LegacyGitCredentialHelper},
		{Key: "credential.useHttpPath", Value: "true"},
	}
	for _, managed := range []bool{false, true} {
		for i := range entries {
			if got := IsManagedGitCredentialConfigEntry(i, entries, managed); got != managed {
				t.Fatalf("entry %d ownership = %v, want %v", i, got, managed)
			}
		}
	}
}
