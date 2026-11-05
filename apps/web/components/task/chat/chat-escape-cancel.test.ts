import { describe, expect, it, vi } from "vitest";
import {
  createComposerEscapeCaptureHandler,
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
