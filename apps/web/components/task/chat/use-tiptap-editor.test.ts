import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { Extension } from "@tiptap/core";
import {
  TIPTAP_EDITOR_TEXT_SIZE_CLASS,
  buildEditorExtensions,
  decideSubmitShortcut,
  shouldRestoreFocusOnEnable,
  useSyncDisabledState,
} from "./use-tiptap-editor";
import * as tiptapEditor from "./use-tiptap-editor";
import { decideHistoryNav } from "./tiptap-editor-history";

describe("TIPTAP_EDITOR_TEXT_SIZE_CLASS", () => {
  it("uses one text size at every viewport width", () => {
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).toContain("text-sm");
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).not.toContain("text-base");
  });

  it("carries no variant-prefixed text utility, so resizing cannot change the font", () => {
    // The touch 16px floor lives in the `any-pointer: coarse` rule in
    // globals.css; a width breakpoint here would resize the composer text when
    // a desktop window is dragged narrow. Reject *any* `<variant>:text-*`
    // rather than a list of named breakpoints — an arbitrary variant such as
    // `min-[1024px]:text-lg` or `max-[900px]:text-base` is just as
    // viewport-dependent and would slip past an enumerated pattern.
    const variantTextUtility = /(?:^|\s)\S+:text-\S+/;
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).not.toMatch(variantTextUtility);
  });
});

describe("editor extensions", () => {
  it("exposes the extension builder for editor-contract verification", () => {
    expect(typeof (tiptapEditor as Record<string, unknown>).buildEditorExtensions).toBe("function");
  });

  it("installs the separate entityReference atom", () => {
    const extensions = buildEditorExtensions({
      mentionSuggestion: {},
      slashSuggestion: {},
      submitKeymap: Extension.create({ name: "submit-test" }),
      historyKeymap: Extension.create({ name: "history-test" }),
    });

    expect(extensions.map((extension) => extension.name)).toContain("entityReference");
  });

  it("registers the # suggestion plugin independently from @ and slash", () => {
    const entityReferenceSuggestion = { char: "#" };
    const build = buildEditorExtensions as unknown as (args: {
      mentionSuggestion: object;
      slashSuggestion: object;
      entityReferenceSuggestion: object;
      submitKeymap: Extension;
      historyKeymap: Extension;
    }) => ReturnType<typeof buildEditorExtensions>;
    const extensions = build({
      mentionSuggestion: { char: "@" },
      slashSuggestion: { char: "/" },
      entityReferenceSuggestion,
      submitKeymap: Extension.create({ name: "submit-test" }),
      historyKeymap: Extension.create({ name: "history-test" }),
    });
    const contextMention = extensions.find((extension) => extension.name === "contextMention");

    expect(contextMention?.options.suggestions).toContain(entityReferenceSuggestion);
  });
});

describe("decideSubmitShortcut", () => {
  describe("submitKey=enter", () => {
    it("submits on Enter when no suggestion menu is open", () => {
      expect(
        decideSubmitShortcut({
          pressed: "enter",
          disabled: false,
          submitKey: "enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("submit");
    });

    // Regression: when slash/@ suggestion popup is open and the user presses
    // Enter to pick the highlighted item, the keymap must defer to the
    // suggestion plugin instead of submitting the message.
    it("defers to the suggestion plugin when the menu is open", () => {
      expect(
        decideSubmitShortcut({
          pressed: "enter",
          disabled: false,
          submitKey: "enter",
          isSuggestionMenuOpen: true,
        }),
      ).toBe("defer");
    });

    it("does not submit on Mod-Enter (Mod-Enter is treated as a newline path)", () => {
      expect(
        decideSubmitShortcut({
          pressed: "mod-enter",
          disabled: false,
          submitKey: "enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("defer");
    });
  });

  describe("submitKey=cmd_enter", () => {
    it("does not submit on Enter — defers (suggestion or newline)", () => {
      expect(
        decideSubmitShortcut({
          pressed: "enter",
          disabled: false,
          submitKey: "cmd_enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("defer");
    });

    it("submits on Mod-Enter when no menu is open", () => {
      expect(
        decideSubmitShortcut({
          pressed: "mod-enter",
          disabled: false,
          submitKey: "cmd_enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("submit");
    });

    // Mod-Enter is not a suggestion-pick key (handleMenuKeyDown only handles
    // Enter and Tab) so the menu state is intentionally ignored — Mod-Enter
    // always submits in cmd_enter mode.
    it("submits on Mod-Enter even when the menu is open", () => {
      expect(
        decideSubmitShortcut({
          pressed: "mod-enter",
          disabled: false,
          submitKey: "cmd_enter",
          isSuggestionMenuOpen: true,
        }),
      ).toBe("submit");
    });
  });

  describe("disabled", () => {
    it("consumes Enter without submitting when the input is disabled", () => {
      expect(
        decideSubmitShortcut({
          pressed: "enter",
          disabled: true,
          submitKey: "enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("consume-noop");
    });

    it("consumes Mod-Enter without submitting when the input is disabled", () => {
      expect(
        decideSubmitShortcut({
          pressed: "mod-enter",
          disabled: true,
          submitKey: "cmd_enter",
          isSuggestionMenuOpen: false,
        }),
      ).toBe("consume-noop");
    });
  });
});

// Regression: after a send, ProseMirror flips `contenteditable` false then
// true. A real browser blurs on the first flip and does not restore focus on
// the second (jsdom does not reproduce this, so the blur is staged
// explicitly here) -- the composer must regain focus unless something else
// has since claimed it.
describe("shouldRestoreFocusOnEnable", () => {
  let input: HTMLInputElement;
  let other: HTMLInputElement;

  afterEach(() => {
    input.remove();
    other.remove();
  });

  function mountInputs() {
    input = document.createElement("input");
    other = document.createElement("input");
    document.body.append(input, other);
  }

  it("restores focus when nothing has since claimed it", () => {
    mountInputs();
    input.focus();
    expect(document.activeElement).toBe(input);
    input.blur();
    expect(document.activeElement).toBe(document.body);

    expect(shouldRestoreFocusOnEnable(true)).toBe(true);
  });

  it("does not restore focus once another element has claimed it", () => {
    mountInputs();
    input.focus();
    input.blur();
    other.focus();
    expect(document.activeElement).toBe(other);

    expect(shouldRestoreFocusOnEnable(true)).toBe(false);
  });

  it("does nothing when the editor never had focus before disabling", () => {
    mountInputs();
    input.blur();
    expect(document.activeElement).toBe(document.body);

    expect(shouldRestoreFocusOnEnable(false)).toBe(false);
  });
});

// Regression: exercises the actual effect wiring, not just the pure
// predicate above -- a mock editor stands in for TipTap's `Editor` since
// jsdom cannot reproduce the browser's disable-blurs-the-element behavior
// the effect exists to work around.
describe("useSyncDisabledState", () => {
  function makeEditor(hasFocus: boolean) {
    return {
      view: { hasFocus: () => hasFocus },
      setEditable: vi.fn(),
      commands: { focus: vi.fn() },
    };
  }

  it("captures focus before disabling, then restores it on re-enable", () => {
    const editor = makeEditor(true);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    expect(editor.setEditable).toHaveBeenLastCalledWith(false);
    expect(editor.commands.focus).not.toHaveBeenCalled();

    rerender({ disabled: false });
    expect(editor.setEditable).toHaveBeenLastCalledWith(true);
    expect(editor.commands.focus).toHaveBeenCalledOnce();
  });

  it("does not steal focus back when another control has since claimed it", () => {
    const editor = makeEditor(true);
    const claimant = document.createElement("input");
    document.body.append(claimant);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    claimant.focus();
    rerender({ disabled: false });

    expect(editor.commands.focus).not.toHaveBeenCalled();
    claimant.remove();
  });

  it("does not focus on re-enable when the editor never had focus before disabling", () => {
    const editor = makeEditor(false);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    rerender({ disabled: false });

    expect(editor.commands.focus).not.toHaveBeenCalled();
  });
});

describe("decideHistoryNav", () => {
  const base = {
    disabled: false,
    isSuggestionMenuOpen: false,
    isReverseSearchOpen: false,
    atBoundary: true,
    historyLength: 3,
    state: { index: null as number | null },
  };

  it("defers when disabled", () => {
    expect(decideHistoryNav({ ...base, direction: "up", disabled: true })).toEqual({
      kind: "defer",
    });
  });

  it("defers when the slash/@ menu is open", () => {
    expect(decideHistoryNav({ ...base, direction: "up", isSuggestionMenuOpen: true })).toEqual({
      kind: "defer",
    });
  });

  it("defers when the reverse-search overlay owns focus", () => {
    expect(decideHistoryNav({ ...base, direction: "up", isReverseSearchOpen: true })).toEqual({
      kind: "defer",
    });
  });

  it("defers when history is empty", () => {
    expect(decideHistoryNav({ ...base, direction: "up", historyLength: 0 })).toEqual({
      kind: "defer",
    });
  });

  it("defers when caret is not at the textblock boundary", () => {
    expect(decideHistoryNav({ ...base, direction: "up", atBoundary: false })).toEqual({
      kind: "defer",
    });
  });

  it("applies index 0 on first ArrowUp", () => {
    expect(decideHistoryNav({ ...base, direction: "up" })).toEqual({
      kind: "apply",
      index: 0,
    });
  });

  it("walks back on subsequent ArrowUp", () => {
    expect(decideHistoryNav({ ...base, direction: "up", state: { index: 1 } })).toEqual({
      kind: "apply",
      index: 2,
    });
  });

  it("consumes ArrowUp at the oldest entry (no cursor escape)", () => {
    expect(decideHistoryNav({ ...base, direction: "up", state: { index: 2 } })).toEqual({
      kind: "consume-noop",
    });
  });

  it("defers ArrowDown when not in history mode (let cursor move normally)", () => {
    expect(decideHistoryNav({ ...base, direction: "down" })).toEqual({
      kind: "defer",
    });
  });

  it("walks forward on ArrowDown while in history", () => {
    expect(decideHistoryNav({ ...base, direction: "down", state: { index: 2 } })).toEqual({
      kind: "apply",
      index: 1,
    });
  });

  it("exits history (restores draft) on ArrowDown from index 0", () => {
    expect(decideHistoryNav({ ...base, direction: "down", state: { index: 0 } })).toEqual({
      kind: "apply",
      index: null,
    });
  });
});
