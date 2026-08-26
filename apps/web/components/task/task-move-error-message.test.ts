import { describe, expect, it } from "vitest";

import { getTaskMoveErrorDetail, getTaskMoveErrorMessage } from "./task-move-error-message";

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

describe("getTaskMoveErrorDetail", () => {
  const title = "Failed to move task";

  it("returns the server reason as the detail", () => {
    expect(getTaskMoveErrorDetail(new Error("task has an active session (RUNNING)"), title)).toBe(
      "task has an active session (RUNNING)",
    );
  });

  it("returns null when the detail would only repeat the title", () => {
    expect(getTaskMoveErrorDetail({ message: "hidden" }, title)).toBeNull();
    expect(getTaskMoveErrorDetail(new Error("   "), title)).toBeNull();
    expect(getTaskMoveErrorDetail(new Error(title), title)).toBeNull();
  });
});
