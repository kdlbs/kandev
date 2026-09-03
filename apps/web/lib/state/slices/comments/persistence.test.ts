import { beforeEach, describe, expect, it } from "vitest";
import type { DiffComment, PlanComment } from "./types";
import {
  COMMENTS_STORAGE_PREFIX,
  listLegacyPlanComments,
  removeAcknowledgedLegacyPlanComment,
} from "./persistence";

const SESSION_ID = "session-1";

function planComment(id: string, text = "Keep this"): PlanComment {
  return {
    id,
    sessionId: SESSION_ID,
    source: "plan",
    text,
    selectedText: "Plan step",
    from: 2,
    to: 11,
    createdAt: "2026-09-02T00:00:00Z",
    status: "pending",
  };
}

function diffComment(): DiffComment {
  return {
    id: "diff-1",
    sessionId: SESSION_ID,
    source: "diff",
    text: "Fix this",
    filePath: "src/app.ts",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "code",
    createdAt: "2026-09-02T00:00:00Z",
    status: "pending",
  };
}

function stored(): unknown[] {
  return JSON.parse(
    window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`) ?? "[]",
  );
}

describe("legacy plan comment persistence", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("lists only readable pending plan rows for the requested sessions", () => {
    window.sessionStorage.setItem(
      `${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify([planComment("plan-1"), diffComment(), { source: "plan", id: "broken" }]),
    );

    expect(listLegacyPlanComments([SESSION_ID])).toEqual([
      { sessionId: SESSION_ID, comment: planComment("plan-1") },
    ]);
  });

  it("removes only the acknowledged row and preserves mixed or malformed entries", () => {
    const acknowledged = planComment("plan-1");
    const remaining = planComment("plan-2", "Same text, separate UUID");
    const malformed = { source: "plan", id: "broken", text: 42 };
    window.sessionStorage.setItem(
      `${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify([acknowledged, diffComment(), malformed, remaining]),
    );

    expect(removeAcknowledgedLegacyPlanComment(SESSION_ID, acknowledged)).toBe(true);
    expect(stored()).toEqual([diffComment(), malformed, remaining]);
  });

  it("does not erase a same-UUID row edited after upload began", () => {
    const uploaded = planComment("plan-1", "Old body");
    const edited = planComment("plan-1", "New body");
    window.sessionStorage.setItem(
      `${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify([edited, diffComment()]),
    );

    expect(removeAcknowledgedLegacyPlanComment(SESSION_ID, uploaded)).toBe(false);
    expect(stored()).toEqual([edited, diffComment()]);
  });
});
