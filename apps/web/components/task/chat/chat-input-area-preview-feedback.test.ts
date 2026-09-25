import { describe, expect, it } from "vitest";
import { shouldShowPreviewFeedbackFallback } from "./chat-input-area";

describe("shouldShowPreviewFeedbackFallback", () => {
  it.each([
    ["without a selected session", { resolvedSessionId: null }],
    ["with a stopped session", { resolvedSessionId: "session-1", isCompleted: true }],
    [
      "with a launch error",
      { resolvedSessionId: "session-1", isFailed: true, launchErrorOwned: true },
    ],
  ])("returns true %s", (_label, state) => {
    expect(
      shouldShowPreviewFeedbackFallback({
        taskId: "task-1",
        itemCount: 1,
        isFailed: false,
        isCompleted: false,
        executorUnavailable: false,
        ...state,
      }),
    ).toBe(true);
  });

  it("returns false while an active session can accept input", () => {
    expect(
      shouldShowPreviewFeedbackFallback({
        taskId: "task-1",
        resolvedSessionId: "session-1",
        itemCount: 1,
        isFailed: false,
        isCompleted: false,
        executorUnavailable: false,
      }),
    ).toBe(false);
  });
});
