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

func TestNormalizeExactFlagFindsQuotedAndBundledTokens(t *testing.T) {
	const skipFlag = "--dangerously-skip-permissions"
	flags := []models.CLIFlag{
		{
			Flag:        "--verbose '" + skipFlag + "' --profile 'two words'",
			Description: "legacy bundle",
			Enabled:     true,
		},
		{Flag: skipFlag, Enabled: false},
	}

	got := NormalizeExactFlag(flags, skipFlag)
	want := []models.CLIFlag{
		{Flag: "--verbose", Description: "legacy bundle", Enabled: true},
		{Flag: skipFlag, Description: "legacy bundle", Enabled: true},
		{Flag: "--profile 'two words'", Description: "legacy bundle", Enabled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeExactFlag() = %+v, want %+v", got, want)
	}

	args, err := Resolve(got)
	if err != nil {
		t.Fatalf("Resolve normalized flags: %v", err)
	}
	wantArgs := []string{"--verbose", skipFlag, "--profile", "two words"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("resolved args = %q, want %q", args, wantArgs)
	}
}
