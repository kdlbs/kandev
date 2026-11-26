package cliflags

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestNormalizeExactFlagCollapsesDuplicatesToEffectiveChoice(t *testing.T) {
	flags := []models.CLIFlag{
		{Flag: "--dangerously-skip-permissions", Description: "old off", Enabled: false},
		{Flag: "--profile", Description: "unrelated", Enabled: true},
		{Flag: "--dangerously-skip-permissions", Description: "new on", Enabled: true},
	}

	got := NormalizeExactFlag(flags, "--dangerously-skip-permissions")
	want := []models.CLIFlag{
		{Flag: "--dangerously-skip-permissions", Description: "new on", Enabled: true},
		{Flag: "--profile", Description: "unrelated", Enabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeExactFlag() = %+v, want %+v", got, want)
	}
}

func TestNormalizeExactFlagPreservesPositionBeforeDelimiter(t *testing.T) {
	flags := []models.CLIFlag{
		{Flag: "--dangerously-skip-permissions", Enabled: true},
		{Flag: "--", Enabled: true},
		{Flag: "prompt", Enabled: true},
	}

	got := NormalizeExactFlag(flags, "--dangerously-skip-permissions")
	if !reflect.DeepEqual(got, flags) {
		t.Fatalf("NormalizeExactFlag() = %+v, want original option and positional order %+v", got, flags)
	}
}
