import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { TaskNavigationCycleDeadlineError } from "@/lib/state/task-navigation-recovery-clock";
import {
  isReadAbort,
  isTemporaryReadError,
  readRetryAfterMs,
  ReadAttemptDeadlineError,
} from "./temporary-read-error";

function abortError() {
  const error = new Error("aborted");
  error.name = "AbortError";
  return error;
}

describe("temporary read classification", () => {
  it.each([
    new TypeError("Failed to fetch"),
    new TaskNavigationCycleDeadlineError(),
    new ReadAttemptDeadlineError(),
    new ApiError("busy", 429, {}),
    new ApiError("busy", 502, {}),
    new ApiError("busy", 503, {}),
    new ApiError("busy", 504, {}),
    new ApiError("busy", 500, { code: "persistence_unavailable" }),
  ])("treats %s as temporary", (error) => {
    expect(isTemporaryReadError(error)).toBe(true);
  });

  it.each([
    new ApiError("denied", 401, {}),
    new ApiError("denied", 403, {}),
    new ApiError("missing", 404, {}),
    new ApiError("invalid", 400, { error_code: "sidebar_query_invalid" }),
    new ApiError("server", 500, {}),
    new SyntaxError("bad payload"),
    abortError(),
    "offline",
  ])("keeps %s permanent or cancelled", (error) => {
    expect(isTemporaryReadError(error)).toBe(false);
  });

  it("exposes server retry delays and recognizes aborts", () => {
    expect(readRetryAfterMs(new ApiError("busy", 503, {}, 7))).toBe(7_000);
    expect(readRetryAfterMs(new Error("offline"))).toBe(0);
    expect(isReadAbort(abortError())).toBe(true);
    expect(isReadAbort(new Error("offline"))).toBe(false);
  });
});
