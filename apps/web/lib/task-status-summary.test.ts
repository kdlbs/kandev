import { describe, it, expect } from "vitest";
import { isNewerStatusSummary, pickFreshestStatusSummary } from "./task-status-summary";
import type { TaskStatusSummary } from "./types/task-status-summary";

function summary(revision: number, queuedPromptCount = 0): TaskStatusSummary {
  return { revision, queued_prompt_count: queuedPromptCount } as TaskStatusSummary;
}

describe("isNewerStatusSummary", () => {
  it("accepts a strictly higher revision", () => {
    expect(isNewerStatusSummary(summary(5), summary(4))).toBe(true);
  });

  it("rejects an equal revision", () => {
    expect(isNewerStatusSummary(summary(4), summary(4))).toBe(false);
  });

  it("rejects a lower revision", () => {
    expect(isNewerStatusSummary(summary(1), summary(4))).toBe(false);
  });

  it("accepts any reading when nothing is cached", () => {
    expect(isNewerStatusSummary(summary(1), undefined)).toBe(true);
    expect(isNewerStatusSummary(summary(1), null)).toBe(true);
  });

  it("treats an absent incoming reading as not newer", () => {
    // HTTP payloads use omitempty: absent means "not carried", not "cleared".
    expect(isNewerStatusSummary(undefined, summary(4))).toBe(false);
    expect(isNewerStatusSummary(null, summary(4))).toBe(false);
    expect(isNewerStatusSummary(undefined, undefined)).toBe(false);
  });
});

describe("pickFreshestStatusSummary", () => {
  it("keeps the cached reading when the incoming one is strictly older", () => {
    const cached = summary(4);
    expect(pickFreshestStatusSummary(summary(1), cached)).toBe(cached);
  });

  it("takes the incoming reading when it is newer", () => {
    const next = summary(9);
    expect(pickFreshestStatusSummary(next, summary(4))).toBe(next);
  });

  it("keeps the cached reading when the incoming one is absent", () => {
    const cached = summary(4);
    expect(pickFreshestStatusSummary(undefined, cached)).toBe(cached);
  });

  it("takes an equal-revision response so a re-stamped queued count is not pinned", () => {
    // buildTaskDTOsWithSessionInfo re-stamps queued_prompt_count from a fresh
    // queue read without incrementing the revision, so an equal-revision
    // response can still carry a newer count than the cache.
    const next = summary(3, 5);
    expect(pickFreshestStatusSummary(next, summary(3, 0))).toBe(next);
  });
});
