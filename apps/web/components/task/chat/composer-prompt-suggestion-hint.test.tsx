import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { detectPlatform, formatShortcut } from "@/lib/keyboard/utils";
import { ComposerPromptSuggestionHint } from "./composer-prompt-suggestion-hint";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true }),
}));

afterEach(cleanup);

function renderHint(submitKey: "enter" | "cmd_enter") {
  render(
    <ComposerPromptSuggestionHint
      suggestion="run the tests"
      submitKey={submitKey}
      onAccept={() => {}}
    />,
  );
  return screen.getByTestId("prompt-suggestion-accept");
}

// @covers AC-UI-PROMPT-SUGGEST-004.1 AC-UI-PROMPT-SUGGEST-004.3
describe("ComposerPromptSuggestionHint", () => {
  it("names the configured modifier shortcut when Enter adds a new line", () => {
    const label = formatShortcut(SHORTCUTS.SUBMIT, detectPlatform());
    expect(renderHint("cmd_enter").textContent).toContain(label);
  });

  it("names Enter when Enter sends", () => {
    const hint = renderHint("enter");
    const modifierLabel = formatShortcut(SHORTCUTS.SUBMIT, detectPlatform());
    expect(hint.textContent).toContain(formatShortcut(SHORTCUTS.SUBMIT_ENTER, detectPlatform()));
    expect(hint.textContent).not.toContain(modifierLabel);
  });
});
