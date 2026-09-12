package move

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEntryOptions_TrimsFieldsAndDoesNotMutateInput(t *testing.T) {
	input := &EntryOptions{
		ResetContext:   true,
		Instructions:   "  reproduce the checkout failure  ",
		SkipStepPrompt: true,
	}

	got, err := NormalizeEntryOptions(input, "")
	if err != nil {
		t.Fatalf("NormalizeEntryOptions() error = %v", err)
	}
	if got == input {
		t.Fatal("NormalizeEntryOptions() returned the input pointer")
	}
	want := EntryOptions{
		ResetContext:   true,
		Instructions:   "reproduce the checkout failure",
		SkipStepPrompt: true,
	}
	if *got != want {
		t.Fatalf("normalized options = %+v, want %+v", *got, want)
	}
	if input.Instructions != "  reproduce the checkout failure  " {
		t.Fatalf("NormalizeEntryOptions() mutated input: %+v", *input)
	}
}

func TestNormalizeEntryOptions_UsesLegacyPromptAlias(t *testing.T) {
	got, err := NormalizeEntryOptions(nil, "  inspect the failing test  ")
	if err != nil {
		t.Fatalf("NormalizeEntryOptions() error = %v", err)
	}
	if got == nil || got.Instructions != "inspect the failing test" {
		t.Fatalf("normalized legacy prompt = %+v, want instructions", got)
	}
}

func TestNormalizeEntryOptions_RejectsLegacyAndNestedPromptWithoutLeakingValues(t *testing.T) {
	const secretNested = "nested private handoff"
	for _, secretLegacy := range []string{secretNested, "legacy private handoff"} {
		_, err := NormalizeEntryOptions(&EntryOptions{Instructions: secretNested}, secretLegacy)
		if err == nil {
			t.Fatal("NormalizeEntryOptions() error = nil, want conflict")
		}
		if !errors.Is(err, ErrConflictingInstructions) {
			t.Fatalf("error = %v, want ErrConflictingInstructions", err)
		}
		var conflict *ConflictingInstructionsError
		if !errors.As(err, &conflict) {
			t.Fatalf("error = %T, want *ConflictingInstructionsError", err)
		}
		if strings.Contains(err.Error(), secretNested) || strings.Contains(err.Error(), secretLegacy) {
			t.Fatalf("conflict error leaked prompt contents: %q", err)
		}
	}
}

func TestNormalizeEntryOptions_BlankValuesBecomeAbsent(t *testing.T) {
	got, err := NormalizeEntryOptions(&EntryOptions{
		Instructions: " \n\t",
	}, " \n")
	if err != nil {
		t.Fatalf("NormalizeEntryOptions() error = %v", err)
	}
	if got != nil {
		t.Fatalf("normalized blank options = %+v, want nil", *got)
	}
	if HasOverrides(&EntryOptions{Instructions: " \t"}) {
		t.Fatal("HasOverrides(blank options) = true, want false")
	}
	if (&EntryOptions{ResetContext: true}).HasOverrides() != true {
		t.Fatal("HasOverrides(reset_context) = false, want true")
	}
}

func TestValidateEntryOptions_RequiresStepChange(t *testing.T) {
	options := &EntryOptions{Instructions: "run QA"}
	for _, change := range []MoveChange{MoveChangeNone, MoveChangePositionOnly} {
		err := ValidateEntryOptions(options, change)
		if err == nil {
			t.Fatalf("ValidateEntryOptions(%q) error = nil, want rejection", change)
		}
		if !errors.Is(err, ErrEntryOptionsRequireStepChange) {
			t.Fatalf("ValidateEntryOptions(%q) error = %v, want ErrEntryOptionsRequireStepChange", change, err)
		}
		var notAllowed *EntryOptionsNotAllowedError
		if !errors.As(err, &notAllowed) {
			t.Fatalf("ValidateEntryOptions(%q) error = %T, want *EntryOptionsNotAllowedError", change, err)
		}
	}
}

func TestValidateEntryOptions_AllowsStepChangeAndEmptyOptions(t *testing.T) {
	if err := ValidateEntryOptions(&EntryOptions{SkipStepPrompt: true}, MoveChangeStep); err != nil {
		t.Fatalf("ValidateEntryOptions(step change) error = %v", err)
	}
	if err := ValidateEntryOptions(nil, MoveChangeNone); err != nil {
		t.Fatalf("ValidateEntryOptions(nil) error = %v", err)
	}
	if err := ValidateEntryOptions(&EntryOptions{}, MoveChangePositionOnly); err != nil {
		t.Fatalf("empty EntryOptions should be a no-op: %v", err)
	}
}

func TestValidateEntryOptions_ErrorDoesNotEchoUnknownChangeValue(t *testing.T) {
	const sensitiveChange = "position-only-with-private-target"
	err := ValidateEntryOptions(&EntryOptions{SkipStepPrompt: true}, MoveChange(sensitiveChange))
	if err == nil {
		t.Fatal("ValidateEntryOptions() error = nil, want rejection")
	}
	if strings.Contains(err.Error(), sensitiveChange) {
		t.Fatalf("validation error leaked change value: %q", err)
	}
}

func TestEntryOptionsJSONRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	for _, encoded := range []string{
		`{"reset_contex":true}`,
		`{"model":"gpt-5"}{"instructions":"unexpected"}`,
	} {
		var options EntryOptions
		if err := json.Unmarshal([]byte(encoded), &options); err == nil {
			t.Fatalf("json.Unmarshal(%s) error = nil, want rejection", encoded)
		}
	}
}
