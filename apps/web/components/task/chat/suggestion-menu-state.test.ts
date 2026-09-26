import { describe, expect, it } from "vitest";
import { getSuggestionMenuOpenState } from "./suggestion-menu-state";

describe("composer suggestion menu state", () => {
  it("keeps an empty mention menu open for Escape handling", () => {
    expect(
      getSuggestionMenuOpenState({
        mentionIsOpen: true,
        slashIsOpen: false,
        slashItemCount: 0,
        entityReferenceMenuOpen: false,
      }),
    ).toEqual({ mentionMenuOpen: true, slashMenuOpen: false, isSuggestionMenuOpen: true });
  });
});
