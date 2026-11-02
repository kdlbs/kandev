import { expect, it } from "vitest";
import type { WalkthroughComment } from "@/lib/state/slices/comments";
import type { TaskWalkthrough } from "@/lib/types/http";
import { getWalkthroughComments } from "./walkthrough-comments";
const time = "2026-10-22T00:00:00Z";
const walkthrough: TaskWalkthrough = {
  id: "tour",
  task_id: "task",
  title: "Tour",
  created_by: "agent",
  created_at: time,
  updated_at: time,
  steps: [
    { file: "a.ts", line: 3, line_end: 1, text: "Explanation", repo: "repo" },
    { file: "b.ts", line: 1, text: "Other" },
  ],
};
const note: WalkthroughComment = {
  id: "a",
  source: "walkthrough",
  sessionId: "session",
  taskId: "task",
  walkthroughId: "tour",
  stepIndex: 0,
  stepCount: 2,
  repo: "repo",
  filePath: "a.ts",
  startLine: 1,
  endLine: 3,
  stepText: "Explanation",
  text: "Feedback",
  createdAt: time,
  status: "pending",
};
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1
it("returns pending and submitted comments in stable creation order", () => {
  const sent = { ...note, id: "b", status: "sent" as const };
  expect(getWalkthroughComments([sent, note], walkthrough, "session", 0).map((c) => c.id)).toEqual([
    "a",
    "b",
  ]);
});
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.2
it.each([
  { taskId: "other" },
  { sessionId: "other" },
  { walkthroughId: "other" },
  { walkthroughId: undefined },
  { stepIndex: 1 },
  { repo: "other" },
  { filePath: "b.ts" },
  { startLine: 2 },
  { endLine: 4 },
  { stepText: "Replaced" },
  { walkthroughUpdatedAt: "2026-10-23T00:00:00Z" },
])("rejects mismatched context %j", (override) => {
  expect(getWalkthroughComments([{ ...note, ...override }], walkthrough, "session", 0)).toEqual([]);
  expect(getWalkthroughComments([note], walkthrough, "session", 0)).toHaveLength(1);
});
it("accepts an exact revision and fails closed for a missing step", () => {
  expect(
    getWalkthroughComments(
      [{ ...note, walkthroughUpdatedAt: time } as WalkthroughComment],
      walkthrough,
      "session",
      0,
    ),
  ).toHaveLength(1);
  expect(getWalkthroughComments([note], walkthrough, "session", 9)).toEqual([]);
});
