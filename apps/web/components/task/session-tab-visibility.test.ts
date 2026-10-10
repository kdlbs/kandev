import { describe, expect, it } from "vitest";
import { countVisibleSessionPanels } from "./session-tab-visibility";

describe("session tab visibility", () => {
  it("counts only panels owned by the active task", () => {
    expect(
      countVisibleSessionPanels(
        [{ id: "session:current" }, { id: "session:stale" }, { id: "chat" }],
        new Set(["current"]),
      ),
    ).toBe(1);
  });
});
