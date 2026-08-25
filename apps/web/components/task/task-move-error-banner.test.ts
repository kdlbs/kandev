import { describe, expect, it } from "vitest";

import { getTaskMoveErrorMessage } from "./task-move-error-banner";

describe("getTaskMoveErrorMessage", () => {
  const fallback = "fallback";
  const activeSessionMessage = "active session";

  it("uses an Error message", () => {
    expect(getTaskMoveErrorMessage(new Error(activeSessionMessage), fallback)).toBe(
      activeSessionMessage,
    );
  });

  it("supports string rejections", () => {
    expect(getTaskMoveErrorMessage(activeSessionMessage, fallback)).toBe(activeSessionMessage);
  });

  it("uses the fallback for empty or unknown errors", () => {
    expect(getTaskMoveErrorMessage(new Error("  "), fallback)).toBe(fallback);
    expect(getTaskMoveErrorMessage({ message: activeSessionMessage }, fallback)).toBe(fallback);
  });
});
