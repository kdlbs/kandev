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
		{Flag: "--profile", Description: "unrelated", Enabled: true},
		{Flag: "--dangerously-skip-permissions", Description: "new on", Enabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeExactFlag() = %+v, want %+v", got, want)
	}
}
