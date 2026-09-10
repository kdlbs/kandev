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
//
// requestAnimationFrame is stubbed with a queue the test drives one frame at
// a time (rather than firing synchronously), so a test can interleave a
// browser-driven blur landing *between* retry attempts -- the scenario that
// made the real fix necessary (a same-tick restore raced a queued blur and
// lost).
let frameQueue: FrameRequestCallback[];

function stubAnimationFrame() {
  frameQueue = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    frameQueue.push(cb);
    return frameQueue.length;
  });
  vi.stubGlobal("cancelAnimationFrame", (handle: number) => {
    frameQueue[handle - 1] = () => {};
  });
}

function flushFrame() {
  const cb = frameQueue.shift();
  cb?.(0);
}

/** Stateful stand-in for TipTap's `Editor`: `view.hasFocus()` reflects
 *  whatever last happened to it, `setEditable(false)` simulates the real
 *  browser's disable-blurs-the-element behavior, and `commands.focus`
 *  simulates a successful restore -- so a test can also simulate a
 *  *delayed* external blur landing after a restore already ran. */
function makeEditor(hasFocusInitially: boolean) {
  let focused = hasFocusInitially;
  return {
    view: { hasFocus: () => focused },
    setEditable: vi.fn((editable: boolean) => {
      if (!editable) focused = false;
    }),
    commands: { focus: vi.fn(() => void (focused = true)) },
    simulateExternalBlur: () => void (focused = false),
  };
}

/** Variant of `makeEditor` where `setEditable(false)` does NOT blur --
 *  modeling the real Chromium behavior the retry loop's comment cites, where
 *  the blur caused by `contenteditable` flipping is applied on a queued task
 *  rather than synchronously. `landQueuedBlur()` fires that queued blur
 *  on demand, independent of `setEditable`, so a test can land it after the
 *  loop has already run one or more attempts. */
function makeQueuedBlurEditor(hasFocusInitially: boolean) {
  let focused = hasFocusInitially;
  return {
    view: { hasFocus: () => focused },
    setEditable: vi.fn(),
    commands: { focus: vi.fn(() => void (focused = true)) },
    landQueuedBlur: () => void (focused = false),
  };
}

describe("useSyncDisabledState", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("captures focus before disabling, then restores it on re-enable", () => {
    stubAnimationFrame();
    const editor = makeEditor(true);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    expect(editor.setEditable).toHaveBeenLastCalledWith(false);
    expect(editor.commands.focus).not.toHaveBeenCalled();

    rerender({ disabled: false });
    expect(editor.setEditable).toHaveBeenLastCalledWith(true);
    expect(editor.commands.focus).not.toHaveBeenCalled();

    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();
    // The retry loop re-checks on the next frame that focus stuck; since it
    // did, it must not call focus() again.
    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();
  });

  it("retries on the next frame when a queued browser blur lands after the first restore", () => {
    stubAnimationFrame();
    const editor = makeEditor(true);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    rerender({ disabled: false });

    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();

    // A blur Chromium queued for the earlier disable lands only now,
    // discarding the restore that already ran.
    editor.simulateExternalBlur();

    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledTimes(2);
  });

  it("stops retrying once another control has since claimed focus", () => {
    stubAnimationFrame();
    const editor = makeEditor(true);
    const claimant = document.createElement("input");
    document.body.append(claimant);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    claimant.focus();
    rerender({ disabled: false });

    flushFrame();
    expect(editor.commands.focus).not.toHaveBeenCalled();
    expect(frameQueue).toHaveLength(0);
    claimant.remove();
  });

  it("does not focus on re-enable when the editor never had focus before disabling", () => {
    stubAnimationFrame();
    const editor = makeEditor(false);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    rerender({ disabled: false });

    expect(editor.commands.focus).not.toHaveBeenCalled();
    expect(frameQueue).toHaveLength(0);
  });

  it("gives up after the retry budget instead of polling forever", () => {
    stubAnimationFrame();
    const editor = makeEditor(true);
    // Focus never sticks and nothing else claims it either -- simulates a
    // pathological case where the editor keeps losing focus every frame.
    editor.commands.focus = vi.fn();
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    rerender({ disabled: false });

    for (let i = 0; i < 10; i += 1) flushFrame();

    expect(editor.commands.focus).toHaveBeenCalledTimes(5);
    expect(frameQueue).toHaveLength(0);
  });

  it("stays armed on frame 1 when the queued browser blur has not landed yet, and restores focus once it does", () => {
    stubAnimationFrame();
    const editor = makeQueuedBlurEditor(true);
    const { rerender } = renderHook(({ disabled }) => useSyncDisabledState(editor, disabled), {
      initialProps: { disabled: false },
    });

    rerender({ disabled: true });
    rerender({ disabled: false });

    // Frame 1: the queued blur has not landed yet, so the editor still
    // reports focus. That is not evidence the restore succeeded -- it is
    // just that the browser hasn't gotten around to the blur it queued.
    flushFrame();
    expect(editor.commands.focus).not.toHaveBeenCalled();

    // The queued blur lands only now.
    editor.landQueuedBlur();

    // The loop must still be armed to catch it.
    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();
    expect(editor.view.hasFocus()).toBe(true);
  });
});

describe("useSyncDisabledState cleanup", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stops retrying after unmount", () => {
    stubAnimationFrame();
    const editor = makeEditor(true);
    editor.commands.focus = vi.fn();
    const { rerender, unmount } = renderHook(
      ({ disabled }) => useSyncDisabledState(editor, disabled),
      { initialProps: { disabled: false } },
    );

    rerender({ disabled: true });
    rerender({ disabled: false });

    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();

    unmount();
    flushFrame();
    expect(editor.commands.focus).toHaveBeenCalledOnce();
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
