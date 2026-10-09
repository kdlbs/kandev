import { beforeEach, expect, it } from "vitest";
import { buildReviewFileComment } from "./review-file";
import { useCommentsStore } from "./comments-store";
import { loadSessionComments } from "./persistence";

const target = {
  sessionId: "s1",
  repositoryName: "packages/lib",
  repositoryId: "repo",
  filePath: "README.md",
  baseRef: "parent-gitlink",
  isSubmodule: true,
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
it("creates whole-file feedback and preserves identity through edit, reload, and deletion", () => {
  const comment = buildReviewFileComment(target, "  Split this file  ");
  expect(comment).toMatchObject({
    ...target,
    source: "review-file",
    text: "Split this file",
    status: "pending",
  });
  expect(comment).not.toHaveProperty("startLine");
  useCommentsStore.getState().addComment(comment!);
  useCommentsStore.getState().updateComment(comment!.id, { text: "Updated" });
  expect(loadSessionComments("s1")).toEqual([
    expect.objectContaining({ ...target, text: "Updated" }),
  ]);
  useCommentsStore.setState({ byId: {}, bySession: {}, pendingForChat: [] });
  useCommentsStore.getState().hydrateSession("s1");
  expect(useCommentsStore.getState().pendingForChat).toEqual([comment!.id]);
  expect(useCommentsStore.getState().getCommentsForFile("s1", "README.md")).toEqual([]);
  useCommentsStore.getState().removeComment(comment!.id);
  expect(loadSessionComments("s1")).toEqual([]);
});
it("rejects blank feedback or a missing session", () => {
  expect(buildReviewFileComment(target, " \n ")).toBeNull();
  expect(buildReviewFileComment({ ...target, sessionId: "" }, "Feedback")).toBeNull();
});
