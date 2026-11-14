import { describe, expect, it } from "vitest";
import {
  capacityGradePatch,
  gradeLimitDrafts,
  type GradeLimitDraft,
} from "./session-capacity-grade-draft";

const saved = {
  enabled: true,
  max_sessions: 5,
  policy: "all" as const,
  grade_limits: { work: 2 },
};

function drafts(count: number): GradeLimitDraft[] {
  return Array.from({ length: count }, (_, index) => ({
    id: String(index),
    grade: `grade-${index}`,
    maximum: "0",
  }));
}

describe("session capacity grade drafts", () => {
  it("accepts the bounded grade map and zero limits", () => {
    const patch = capacityGradePatch(saved, true, "all", drafts(64));

    expect(Object.keys(patch?.grade_limits ?? {})).toHaveLength(64);
    expect(patch?.grade_limits?.["grade-0"]).toBe(0);
  });

  it("rejects more grade limits than the backend accepts", () => {
    expect(capacityGradePatch(saved, true, "all", drafts(65))).toBeNull();
  });

  it("restoring saved grade rows produces no patch", () => {
    expect(capacityGradePatch(saved, true, "all", gradeLimitDrafts(saved.grade_limits))).toEqual(
      {},
    );
  });
});
