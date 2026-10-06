package agents

import (
	"strings"
	"testing"
)

func TestOpenCodeRuntimeFamiliesHaveReviewedMajorPins(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		wantMajor   string
	}{
		{name: "v1", packageName: "opencode-ai", wantMajor: "1"},
		{name: "v2", packageName: "@opencode/cli", wantMajor: "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := DefaultManagedNPMRuntimeVersion(tt.packageName)
			if err != nil {
				t.Fatalf("DefaultManagedNPMRuntimeVersion(%q): %v", tt.packageName, err)
			}
			major, _, _ := strings.Cut(version, ".")
			if major != tt.wantMajor {
				t.Fatalf("default version for %q = %q, want major %s", tt.packageName, version, tt.wantMajor)
			}
		})
	}
}
