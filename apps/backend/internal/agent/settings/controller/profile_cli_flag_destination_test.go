package controller

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.3, .5
// Over ACP the launched process is the bridge, which forwards no unrecognized
// argument to the CLI it wraps. Accepting the flag there changed nothing while
// the editor reported it as enabled.
func TestPassthroughOnlyFlagRejectedOnACPProfile(t *testing.T) {
	err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: true}},
		false,
	)

	if !errors.Is(err, ErrPassthroughOnlyCLIFlag) {
		t.Fatalf("err = %v, want ErrPassthroughOnlyCLIFlag", err)
	}
	// A refusal without a next step is not actionable.
	if got := err.Error(); !contains(got, "permission mode") {
		t.Fatalf("error = %q, want it to name the ACP equivalent", got)
	}
}

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-001.6
func TestPassthroughOnlyFlagAllowedOnPassthroughProfile(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: true}},
		true,
	); err != nil {
		t.Fatalf("err = %v, want the flag accepted in passthrough mode", err)
	}
}

// A disabled entry is not applied to any launch, so it is not refused.
func TestDisabledPassthroughOnlyFlagIsAccepted(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--dangerously-skip-permissions", Enabled: false}},
		false,
	); err != nil {
		t.Fatalf("err = %v, want a disabled entry accepted", err)
	}
}

// The check judges resolved tokens, so a restricted flag cannot hide behind
// its neighbours in a multi-token entry.
func TestPassthroughOnlyFlagDetectedInMultiTokenEntry(t *testing.T) {
	err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--verbose --dangerously-skip-permissions", Enabled: true}},
		false,
	)
	if !errors.Is(err, ErrPassthroughOnlyCLIFlag) {
		t.Fatalf("err = %v, want the restricted token found among its neighbours", err)
	}
}

// Unrestricted flags keep working; they legitimately configure the launched
// bridge process.
func TestUnrestrictedFlagsRemainAccepted(t *testing.T) {
	if err := validatePassthroughOnlyCLIFlags(
		agents.NewClaudeACP(),
		[]dto.CLIFlagDTO{{Flag: "--verbose", Enabled: true}},
		false,
	); err != nil {
		t.Fatalf("err = %v, want an unrestricted flag accepted", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// A partial update that only turns CLI passthrough off used to skip validation
// entirely, because it carried no cli_flags. The profile then kept an enabled
// flag that cannot reach the agent over ACP. The saved list is what must be
// judged, so the stored flags have to survive the conversion with Enabled
// intact.
func TestStoredFlagsAreJudgedWhenPassthroughTurnsOff(t *testing.T) {
	stored := []models.CLIFlag{
		{Flag: "--verbose", Enabled: true},
		{Flag: "--dangerously-skip-permissions", Enabled: true},
	}

	err := validatePassthroughOnlyCLIFlags(agents.NewClaudeACP(), cliFlagsToDTO(stored), false)

	if !errors.Is(err, ErrPassthroughOnlyCLIFlag) {
		t.Fatalf("err = %v, want ErrPassthroughOnlyCLIFlag for a retained flag", err)
	}
}

// The same retained list stays acceptable while the profile keeps passthrough.
func TestStoredFlagsRemainAcceptedWhilePassthroughStaysOn(t *testing.T) {
	stored := []models.CLIFlag{{Flag: "--dangerously-skip-permissions", Enabled: true}}

	if err := validatePassthroughOnlyCLIFlags(agents.NewClaudeACP(), cliFlagsToDTO(stored), true); err != nil {
		t.Fatalf("err = %v, want the retained flag accepted in passthrough mode", err)
	}
}
