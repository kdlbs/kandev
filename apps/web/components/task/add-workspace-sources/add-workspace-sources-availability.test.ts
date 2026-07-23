import { describe, expect, it } from "vitest";
import {
  getAddSourcesDisabledReason,
  hasActiveTaskSourceWork,
} from "./add-workspace-sources-availability";

describe("getAddSourcesDisabledReason", () => {
  it("explains loading before the Files action can safely use task sources", () => {
    expect(getAddSourcesDisabledReason({ isLoading: true })).toBe(
      "Wait for task sources to finish loading before adding sources.",
    );
  });

  it("explains an active turn or tool call", () => {
    expect(getAddSourcesDisabledReason({ hasActiveTurn: true })).toBe(
      "Wait for the active turn or tool call to finish before adding sources.",
    );
  });

  it("blocks source changes when any task session has active work", () => {
    expect(
      hasActiveTaskSourceWork(["primary", "subtask"], {
        primary: null,
        subtask: "turn-2",
        unrelated: "turn-3",
      }),
    ).toBe(true);
  });

  it("does not treat an unrelated session as task work", () => {
    expect(hasActiveTaskSourceWork(["primary"], { unrelated: "turn-3" })).toBe(false);
  });
});
