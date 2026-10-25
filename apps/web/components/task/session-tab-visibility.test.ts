import { describe, expect, it } from "vitest";
import {
  countVisibleSessionPanels,
  shouldShowSessionTabCloseAction,
} from "./session-tab-visibility";

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

// @covers AC-TASKS-SESSION-TAB-RESTART-001.4
describe("session tab close visibility", () => {
  const state = { sessionCount: 2, visibleSessionCount: 2, isRestarting: false };

  it.each(["WAITING_FOR_INPUT", "CANCELLED", "COMPLETED", "FAILED"] as const)(
    "blocks deletion only while restart is pending for %s",
    (sessionState) => {
      expect(shouldShowSessionTabCloseAction("delete_session", sessionState, state)).toBe(true);
      expect(
        shouldShowSessionTabCloseAction("delete_session", sessionState, {
          ...state,
          isRestarting: true,
        }),
      ).toBe(false);
    },
  );

  it.each(["RUNNING", "STARTING", null] as const)(
    "keeps deletion unavailable for an ineligible state: %s",
    (sessionState) => {
      expect(shouldShowSessionTabCloseAction("delete_session", sessionState, state)).toBe(false);
    },
  );

  it("keeps hide-panel close available during restart without hiding the last visible tab", () => {
    const restarting = { ...state, isRestarting: true };
    expect(shouldShowSessionTabCloseAction("hide_panel", "RUNNING", restarting)).toBe(true);
    expect(
      shouldShowSessionTabCloseAction("hide_panel", "RUNNING", {
        ...restarting,
        visibleSessionCount: 1,
      }),
    ).toBe(false);
  });

  it("keeps the last session unavailable for deletion", () => {
    expect(
      shouldShowSessionTabCloseAction("delete_session", "WAITING_FOR_INPUT", {
        ...state,
        sessionCount: 1,
      }),
    ).toBe(false);
  });
});
