import { cleanup, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSuggestionEscapeFallback } from "./use-suggestion-escape-fallback";

afterEach(cleanup);

function renderFallback(overrides: { isSuggestionMenuOpen: boolean; mentionMenuOpen?: boolean }) {
  const closeMentionMenu = vi.fn();
  const closeSlashMenu = vi.fn();
  const closeEntityReferenceMenu = vi.fn();
  const { rerender } = renderHook(
    (props: { isSuggestionMenuOpen: boolean; mentionMenuOpen: boolean }) =>
      useSuggestionEscapeFallback({
        isSuggestionMenuOpen: props.isSuggestionMenuOpen,
        mentionMenuOpen: props.mentionMenuOpen,
        slashMenuOpen: false,
        entityReferenceMenuOpen: false,
        closeMentionMenu,
        closeSlashMenu,
        closeEntityReferenceMenu,
      }),
    {
      initialProps: {
        isSuggestionMenuOpen: overrides.isSuggestionMenuOpen,
        mentionMenuOpen: overrides.mentionMenuOpen ?? false,
      },
    },
  );
  return { closeMentionMenu, closeSlashMenu, closeEntityReferenceMenu, rerender };
}

describe("useSuggestionEscapeFallback", () => {
  it("does nothing on Escape while no suggestion menu is open", () => {
    const { closeMentionMenu, closeSlashMenu, closeEntityReferenceMenu } = renderFallback({
      isSuggestionMenuOpen: false,
    });

    fireEvent.keyDown(document, { key: "Escape" });

    expect(closeMentionMenu).not.toHaveBeenCalled();
    expect(closeSlashMenu).not.toHaveBeenCalled();
    expect(closeEntityReferenceMenu).not.toHaveBeenCalled();
  });

  it("closes the open menu and claims the event when a suggestion menu is open", () => {
    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
    });

    const event = fireEvent.keyDown(document, { key: "Escape", cancelable: true, bubbles: true });

    expect(closeMentionMenu).toHaveBeenCalledTimes(1);
    // fireEvent's return value is the pre-dispatch continue flag: `false` means
    // something called preventDefault() during dispatch.
    expect(event).toBe(false);
  });

  it("ignores non-Escape keys", () => {
    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
    });

    fireEvent.keyDown(document, { key: "Enter" });

    expect(closeMentionMenu).not.toHaveBeenCalled();
  });

  /**
   * On the main task chat panel there is no Radix capture-phase interceptor,
   * so ProseMirror's Suggestion plugin runs its own onKeyDown and calls
   * event.stopPropagation() on the keydown at the editor's own bubble-phase
   * listener -- see tiptap-suggestion.tsx. That halts propagation before the
   * event reaches `document`, so this hook's document-bubble listener never
   * fires there, regardless of registration order (bubble phase always visits
   * the DOM tree from target to document). This test reproduces that halt with
   * a real DOM listener attached between the target and document, using
   * jsdom's genuine event propagation rather than a synthetic stand-in.
   */
  it("stays inert when an earlier bubble-phase listener stops propagation, as on the main task chat panel", () => {
    const editor = document.createElement("div");
    document.body.appendChild(editor);
    const stopAtEditor = (event: KeyboardEvent) => {
      if (event.key === "Escape") event.stopPropagation();
    };
    editor.addEventListener("keydown", stopAtEditor);

    const { closeMentionMenu } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
    });

    fireEvent.keyDown(editor, { key: "Escape", bubbles: true, cancelable: true });

    expect(closeMentionMenu).not.toHaveBeenCalled();

    editor.removeEventListener("keydown", stopAtEditor);
    document.body.removeChild(editor);
  });

  it("removes its listener when the suggestion menu closes", () => {
    const { closeMentionMenu, rerender } = renderFallback({
      isSuggestionMenuOpen: true,
      mentionMenuOpen: true,
    });

    rerender({ isSuggestionMenuOpen: false, mentionMenuOpen: false });
    fireEvent.keyDown(document, { key: "Escape" });

    expect(closeMentionMenu).not.toHaveBeenCalled();
  });
});
