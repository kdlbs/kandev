import { describe, expect, it } from "vitest";
import {
  getSessionRestartReviewStore,
  requestSessionRestartReview,
  settleSessionRestartReview,
} from "./session-restart-review";

const target = { taskId: "task", sessionId: "session", attemptId: "attempt", identity: "operator" };

describe("operator restart review ownership", () => {
  it("waits for an explicit choice and leaves other application stores isolated", async () => {
    const owner = {};
    const other = {};
    let settled = false;
    const pending = requestSessionRestartReview(owner, target).then((value) => {
      settled = true;
      return value;
    });
    await Promise.resolve();
    expect(settled).toBe(false);
    expect(getSessionRestartReviewStore(other).getState()).toBeNull();
    const request = getSessionRestartReviewStore(owner).getState()!;
    expect(request).toEqual(target);
    settleSessionRestartReview(owner, request, { mode: "captured" });
    expect(await pending).toEqual({ mode: "captured" });
    expect(getSessionRestartReviewStore(owner).getState()).toBeNull();
  });

  it("rejects concurrent reviews and stale confirmations", async () => {
    const owner = {};
    const first = requestSessionRestartReview(owner, target);
    const request = getSessionRestartReviewStore(owner).getState()!;
    expect(await requestSessionRestartReview(owner, { ...target, sessionId: "other" })).toBeNull();
    settleSessionRestartReview(
      owner,
      { ...request },
      { mode: "current", selectionDigest: "stale" },
    );
    expect(getSessionRestartReviewStore(owner).getState()).toBe(request);
    settleSessionRestartReview(owner, request, null);
    expect(await first).toBeNull();
    const next = requestSessionRestartReview(owner, target);
    const replacement = getSessionRestartReviewStore(owner).getState()!;
    settleSessionRestartReview(owner, request, { mode: "captured" });
    expect(getSessionRestartReviewStore(owner).getState()).toBe(replacement);
    settleSessionRestartReview(owner, replacement, {
      mode: "current",
      selectionDigest: "reviewed",
    });
    expect(await next).toEqual({ mode: "current", selectionDigest: "reviewed" });
  });
});
