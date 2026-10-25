import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";

import { getTaskMoveErrorDetail, getTaskMoveErrorMessage } from "./task-move-error-message";

const activeSessionError = "task has an active session";

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

  it("translates a structured active-session conflict", () => {
    const translate = (key: string) => `translated:${key}`;
    const error = new ApiError(activeSessionError, 409, {
      error: activeSessionError,
      code: "task_move_active_session",
    });

    expect(getTaskMoveErrorMessage(error, fallback, translate)).toBe(
      "translated:task:taskMoveErrorActiveSession",
    );
  });

  it("uses a translated generic message for an unknown API error", () => {
    const translate = (key: string) => `translated:${key}`;
    const error = new ApiError("internal details", 500, {
      error: "internal details",
      code: "unexpected_move_error",
    });

    expect(getTaskMoveErrorMessage(error, fallback, translate)).toBe(
      "translated:task:taskMoveErrorGeneric",
    );
  });
});

describe("getTaskMoveErrorDetail", () => {
  const title = "Failed to move task";

  it("returns the server reason as the detail", () => {
    expect(getTaskMoveErrorDetail(new Error("task has an active session (RUNNING)"), title)).toBe(
      "task has an active session (RUNNING)",
    );
  });

  it("uses the translated conflict detail", () => {
    const error = new ApiError(activeSessionError, 409, {
      error: activeSessionError,
      code: "task_move_active_session",
    });
    const translate = (key: string) => `translated:${key}`;

    expect(getTaskMoveErrorDetail(error, title, translate)).toBe(
      "translated:task:taskMoveErrorActiveSession",
    );
  });

  it("returns null when the detail would only repeat the title", () => {
    expect(getTaskMoveErrorDetail({ message: "hidden" }, title)).toBeNull();
    expect(getTaskMoveErrorDetail(new Error("   "), title)).toBeNull();
    expect(getTaskMoveErrorDetail(new Error(title), title)).toBeNull();
  });
});
