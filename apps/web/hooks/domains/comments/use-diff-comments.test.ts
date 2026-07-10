import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useCommentsStore, type DiffComment } from "@/lib/state/slices/comments";
import { useDiffFileComments } from "./use-diff-comments";

const SESSION_ID = "session-1";
const FILE_PATH = "README.md";

function diffComment(overrides: Partial<DiffComment>): DiffComment {
  return {
    id: "comment-" + Math.random().toString(36).slice(2),
    sessionId: SESSION_ID,
    source: "diff",
    text: "tighten this",
    filePath: FILE_PATH,
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "hello",
    createdAt: new Date().toISOString(),
    status: "pending",
    ...overrides,
  };
}

describe("useDiffFileComments", () => {
  beforeEach(() => {
    sessionStorage.clear();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("filters comments by repositoryId when one is provided", () => {
    act(() => {
      const store = useCommentsStore.getState();
      store.addComment(diffComment({ id: "repo-a", repositoryId: "repo-a" }));
      store.addComment(diffComment({ id: "repo-b", repositoryId: "repo-b" }));
      store.addComment(diffComment({ id: "legacy" }));
    });

    const { result } = renderHook(() => useDiffFileComments(SESSION_ID, FILE_PATH, "repo-a"));

    expect(result.current.map((comment) => comment.id)).toEqual(["repo-a", "legacy"]);
  });

  it("preserves legacy unscoped reads when repositoryId is omitted", () => {
    act(() => {
      const store = useCommentsStore.getState();
      store.addComment(diffComment({ id: "repo-a", repositoryId: "repo-a" }));
      store.addComment(diffComment({ id: "repo-b", repositoryId: "repo-b" }));
    });

    const { result } = renderHook(() => useDiffFileComments(SESSION_ID, FILE_PATH));

    expect(result.current.map((comment) => comment.id)).toEqual(["repo-a", "repo-b"]);
  });
});
