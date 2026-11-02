package managedruntime

import (
	"slices"
	"testing"
)

func TestOpenCodeCredentialCommandRejectsOtherOperations(t *testing.T) {
	runtime := OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceManaged, Version: "2.1.2"}
	if _, err := runtime.CredentialCommand("logout"); err == nil {
		t.Fatal("credential provisioning admitted an unrelated auth operation")
	}
}

// @covers AC-EXECUTORS-OPENCODE-AUTH-001.9
func TestOpenCodeCredentialRuntimeMetadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime OpenCodeRuntime
		valid   bool
	}{
		{"managed minimum", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceManaged, Version: "2.1.2"}, true},
		{"managed newer", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceManaged, Version: "2.2.1"}, true},
		{"native", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceNative, Version: "2.2.1"}, false},
		{"custom", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: "custom", Version: "2.2.1"}, false},
		{"old", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceManaged, Version: "2.1.1"}, false},
		{"unpinned", OpenCodeRuntime{Family: OpenCodeFamilyV2, Source: OpenCodeSourceManaged, Version: "latest"}, false},
		{"v1", OpenCodeRuntime{Family: OpenCodeFamilyV1, Source: OpenCodeSourceManaged, Version: "1.18.32"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.runtime.CredentialCommand("export")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
			if tc.valid {
				want := []string{"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, "@opencode/cli@" + tc.runtime.Version, "auth", "export", "--standalone"}
				if !slices.Equal(got, want) {
					t.Fatalf("command=%v want=%v", got, want)
				}
			}
		})
	}
}
