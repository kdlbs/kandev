import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { useCommentsStore, type ReviewFileComment } from "@/lib/state/slices/comments";
import { usePendingReviewCommentsByFile } from "./use-review-comments";

beforeEach(() => {
  sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
});

it("keeps whole-file feedback scoped to session and nested repository", () => {
  const make = (id: string, repositoryName: string, sessionId = "s1"): ReviewFileComment => ({
    id,
    source: "review-file",
    sessionId,
    repositoryName,
    repositoryId: "shared-id",
    filePath: "README.md",
    text: id,
    createdAt: "2026-09-17T00:00:00Z",
    status: "pending",
  });
  act(() => {
    for (const c of [make("root", ""), make("nested", "lib"), make("other", "lib", "s2")]) {
      useCommentsStore.getState().addComment(c);
    }
  });
  const { result } = renderHook(() => usePendingReviewCommentsByFile("s1"));
  expect(Object.values(result.current).map((group) => group.map((c) => c.id))).toEqual([
    ["root"],
    ["nested"],
  ]);
});

it("combines root file and line feedback in one composer group", () => {
  act(() => {
    useCommentsStore.getState().addComment({
      id: "line",
      source: "diff",
      sessionId: "s1",
      filePath: "a.txt",
      text: "line",
      status: "pending",
      createdAt: "now",
      startLine: 1,
      endLine: 1,
      side: "additions",
      codeContent: "a",
    });
    useCommentsStore.getState().addComment({
      id: "file",
      source: "review-file",
      sessionId: "s1",
      filePath: "a.txt",
      repositoryName: "",
      text: "file",
      status: "pending",
      createdAt: "now",
    });
  });
  const { result } = renderHook(() => usePendingReviewCommentsByFile("s1"));
  expect(Object.values(result.current)).toHaveLength(1);
  expect(Object.values(result.current)[0].map((c) => c.id)).toEqual(["line", "file"]);
});
