import { describe, expect, it } from "vitest";
import { decideSubmitShortcut } from "./use-tiptap-editor";

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
