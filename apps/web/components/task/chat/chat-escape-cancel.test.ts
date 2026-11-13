import { describe, expect, it, vi } from "vitest";
import {
  createComposerEscapeCaptureHandler,
  shouldCancelAgentWithEscape,
  shouldClaimComposerEscape,
  shouldDeferComposerEscapeToExistingControls,
} from "./chat-escape-cancel";

describe("shouldDeferComposerEscapeToExistingControls", () => {
  it.each([
    { isSuggestionMenuOpen: true, isReverseSearchOpen: false, isPromptSuggestionVisible: false },
    { isSuggestionMenuOpen: false, isReverseSearchOpen: true, isPromptSuggestionVisible: false },
    { isSuggestionMenuOpen: false, isReverseSearchOpen: false, isPromptSuggestionVisible: true },
  ])("preserves existing Escape handling when an editor control is open: %o", (input) => {
    expect(shouldDeferComposerEscapeToExistingControls(input)).toBe(true);
  });

  it("allows cancellation when no editor control owns Escape", () => {
    expect(
      shouldDeferComposerEscapeToExistingControls({
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: false,
      }),
    ).toBe(false);
  });

  it("leaves Escape to an open editor control instead of cancelling the turn", () => {
    const onEscape = vi.fn();
    const handler = createComposerEscapeCaptureHandler(onEscape, true, false, false);

    handler({} as React.KeyboardEvent<HTMLElement>);

    expect(onEscape).not.toHaveBeenCalled();
  });
});

describe("composer Escape cancellation eligibility", () => {
  const active = {
    canCancelAgent: true,
    escapeCancelsAgent: true,
    hasPendingClarification: false,
  };

  it("claims Escape only for an eligible focused composer", () => {
    expect(
      shouldClaimComposerEscape(true, true, {
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: false,
      }),
    ).toBe(true);
  });

  it.each([
    {
      targetIsInsideEditor: false,
      shouldHandleEscape: true,
      controls: {
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: false,
      },
    },
    {
      targetIsInsideEditor: true,
      shouldHandleEscape: false,
      controls: {
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: false,
      },
    },
    {
      targetIsInsideEditor: true,
      shouldHandleEscape: true,
      controls: {
        isSuggestionMenuOpen: true,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: false,
      },
    },
    {
      targetIsInsideEditor: true,
      shouldHandleEscape: true,
      controls: {
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: true,
        isPromptSuggestionVisible: false,
      },
    },
    {
      targetIsInsideEditor: true,
      shouldHandleEscape: true,
      controls: {
        isSuggestionMenuOpen: false,
        isReverseSearchOpen: false,
        isPromptSuggestionVisible: true,
      },
    },
  ])(
    "does not claim Escape outside the eligible composer state: %o",
    ({ targetIsInsideEditor, shouldHandleEscape, controls }) => {
      expect(shouldClaimComposerEscape(targetIsInsideEditor, shouldHandleEscape, controls)).toBe(
        false,
      );
    },
  );

  it.each([
    { key: "Enter" },
    { key: "Escape", isComposing: true },
    { key: "Escape", keyCode: 229 },
    { key: "Escape", repeat: true },
  ])("does not cancel for a different or non-cancelable Escape event: %o", (init) => {
    expect(shouldCancelAgentWithEscape(new KeyboardEvent("keydown", init), active)).toBe(false);
  });

  it("does not cancel an event that another handler prevented", () => {
    const handled = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    handled.preventDefault();
    expect(shouldCancelAgentWithEscape(handled, active)).toBe(false);
  });

  it("leaves Escape to a pending clarification", () => {
    const event = new KeyboardEvent("keydown", { key: "Escape" });
    expect(shouldCancelAgentWithEscape(event, { ...active, hasPendingClarification: true })).toBe(
      false,
    );
  });
});
