package controller

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestRuntimeVersionTargetHelpersAcceptBuildMetadata(t *testing.T) {
	tests := []struct {
		version   string
		nextPatch string
		nextMinor string
	}{
		{version: "1.19.5", nextPatch: "1.19.7", nextMinor: "1.20.0"},
		{version: "1.19.5+build", nextPatch: "1.19.7", nextMinor: "1.20.0"},
		{version: "1.19.5+build.7", nextPatch: "1.19.7", nextMinor: "1.20.0"},
		{version: "2.2.5+linux.amd64", nextPatch: "2.2.7", nextMinor: "2.3.0"},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			original, err := managedruntime.ParseStableVersion(test.version)
			if err != nil {
				t.Fatalf("parse original version: %v", err)
			}
			assertNewerSameMajorVersion(t, original.Major(), test.version, nextPatchVersion(t, test.version), test.nextPatch)
			assertNewerSameMajorVersion(t, original.Major(), test.version, nextRuntimeMinorVersion(t, test.version), test.nextMinor)
		})
	}
}

func assertNewerSameMajorVersion(t *testing.T, major uint64, original, target, want string) {
	t.Helper()
	if target != want {
		t.Fatalf("target for %s = %q, want %q", original, target, want)
	}
	parsed, err := managedruntime.ParseStableVersion(target)
	if err != nil {
		t.Fatalf("parse target %q: %v", target, err)
	}
	if parsed.Major() != major {
		t.Fatalf("target %q changed major from %d to %d", target, major, parsed.Major())
	}
	originalParsed, err := managedruntime.ParseStableVersion(original)
	if err != nil {
		t.Fatalf("parse original version %q: %v", original, err)
	}
	if parsed.Compare(originalParsed) <= 0 {
		t.Fatalf("target %q is not newer than %q", target, original)
	}
}
