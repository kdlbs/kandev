import { beforeEach, expect, it } from "vitest";
import { useCommentsStore } from "./comments-store";
import { loadSessionComments } from "./persistence";
import type { WalkthroughComment } from "./types";

const comment: WalkthroughComment = {
  id: "note",
  source: "walkthrough",
  sessionId: "session",
  taskId: "task",
  walkthroughId: "tour",
  stepIndex: 0,
  stepCount: 2,
  filePath: "a.ts",
  startLine: 1,
  endLine: 1,
  stepText: "Explanation",
  text: "Check this",
  createdAt: "2026-10-22T00:00:00Z",
  status: "pending",
};
beforeEach(() => {
  sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
});
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.3
it("retains acknowledged walkthrough history while removing other comment sources", () => {
  const store = useCommentsStore.getState();
  store.addComment(comment);
  store.addComment({
    id: "diff",
    source: "diff",
    sessionId: "session",
    text: "Other",
    createdAt: comment.createdAt,
    status: "pending",
    filePath: "a.ts",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "code",
  });
  store.addComment({ ...comment, id: "later" });
  store.markCommentsSent(["note", "diff"]);
  store.markCommentsSent(["note"]);
  expect(useCommentsStore.getState().byId.note).toEqual({ ...comment, status: "sent" });
  expect(useCommentsStore.getState().byId.diff).toBeUndefined();
  expect(useCommentsStore.getState().pendingForChat).toEqual(["later"]);
  expect(loadSessionComments("session").map((c) => c.id)).toEqual(["note", "later"]);
});
// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.5
it("restores history without resending it and clears the complete session", () => {
  const store = useCommentsStore.getState();
  store.addComment(comment);
  store.markCommentsSent([comment.id]);
  useCommentsStore.setState({ byId: {}, bySession: {}, pendingForChat: [] });
  store.hydrateSession("session");
  expect(useCommentsStore.getState().byId.note?.status).toBe("sent");
  store.addToPending("note");
  expect(store.getPendingComments()).toEqual([]);
  store.clearSessionComments("session");
  expect(useCommentsStore.getState().byId).toEqual({});
  expect(loadSessionComments("session")).toEqual([]);
});
it("excludes sent history even if a stale pending index contains it", () => {
  useCommentsStore.getState().addComment({ ...comment, status: "sent" });
  useCommentsStore.setState({ pendingForChat: ["note"] });
  expect(useCommentsStore.getState().getPendingComments()).toEqual([]);
});
