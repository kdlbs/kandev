import { describe, expect, it } from "vitest";
import { Extension } from "@tiptap/core";
import {
  TIPTAP_EDITOR_TEXT_SIZE_CLASS,
  buildEditorExtensions,
  decideSubmitShortcut,
} from "./use-tiptap-editor";
import * as tiptapEditor from "./use-tiptap-editor";
import { decideHistoryNav } from "./tiptap-editor-history";

describe("TIPTAP_EDITOR_TEXT_SIZE_CLASS", () => {
  it("keeps 16px text until the lg breakpoint", () => {
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).toContain("text-base");
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).toContain("lg:text-sm");
    expect(TIPTAP_EDITOR_TEXT_SIZE_CLASS).not.toContain("md:text-sm");
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
