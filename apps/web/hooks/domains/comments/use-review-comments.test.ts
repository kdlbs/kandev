import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { useCommentsStore, type ReviewFileComment } from "@/lib/state/slices/comments";
import { usePendingReviewCommentsByFile } from "./use-review-comments";

const REVIEW_FILE_SOURCE = "review-file" as const;

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
    source: REVIEW_FILE_SOURCE,
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
      repositoryName: "",
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
      source: REVIEW_FILE_SOURCE,
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

it("combines scoped line and whole-file feedback in one named composer group", () => {
  act(() => {
    useCommentsStore.getState().addComment({
      id: "line",
      source: "diff",
      repositoryName: "api",
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
      source: REVIEW_FILE_SOURCE,
      sessionId: "s1",
      filePath: "a.txt",
      repositoryName: "api",
      text: "file",
      status: "pending",
      createdAt: "now",
    });
  });
  const { result } = renderHook(() => usePendingReviewCommentsByFile("s1"));
  expect(Object.values(result.current)).toHaveLength(1);
  expect(Object.values(result.current)[0].map((c) => c.id)).toEqual(["line", "file"]);
});

it.each([undefined, "old-repo"])(
  "keeps legacy scope %j separate from explicit root feedback",
  (repositoryId) => {
    const legacy = {
      id: "legacy",
      source: "diff" as const,
      sessionId: "s1",
      repositoryId,
      filePath: "README.md",
      text: "Legacy feedback",
      status: "pending" as const,
      createdAt: "now",
      startLine: 1,
      endLine: 1,
      side: "additions" as const,
      codeContent: "text",
    };
    const root: ReviewFileComment = {
      id: "root",
      source: REVIEW_FILE_SOURCE,
      sessionId: "s1",
      repositoryName: "",
      filePath: "README.md",
      text: "Root feedback",
      status: "pending",
      createdAt: "now",
    };
    act(() => {
      useCommentsStore.getState().addComment(legacy);
      useCommentsStore.getState().addComment(root);
    });
    const { result } = renderHook(() => usePendingReviewCommentsByFile("s1"));
    expect(Object.values(result.current).map((group) => group.map((c) => c.id))).toEqual([
      ["legacy"],
      ["root"],
    ]);
  },
);

it("bridges legacy ID-only feedback only through an unambiguous named group", () => {
  const legacy = {
    id: "legacy",
    source: "diff" as const,
    sessionId: "s1",
    repositoryId: "repo",
    filePath: "README.md",
    text: "Legacy feedback",
    status: "pending" as const,
    createdAt: "now",
    startLine: 1,
    endLine: 1,
    side: "additions" as const,
    codeContent: "text",
  };
  const named: ReviewFileComment = {
    id: "named",
    source: REVIEW_FILE_SOURCE,
    sessionId: "s1",
    repositoryName: "api",
    repositoryId: "repo",
    filePath: "README.md",
    text: "Named feedback",
    status: "pending",
    createdAt: "now",
  };
  act(() => {
    useCommentsStore.getState().addComment(legacy);
    useCommentsStore.getState().addComment(named);
  });
  const { result } = renderHook(() => usePendingReviewCommentsByFile("s1"));
  expect(Object.values(result.current)).toHaveLength(1);
  expect(
    Object.values(result.current)[0].every((comment) => comment.repositoryName === "api"),
  ).toBe(true);
  expect(useCommentsStore.getState().byId.legacy).not.toHaveProperty("repositoryName");
});
