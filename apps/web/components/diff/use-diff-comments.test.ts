import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { useDiffComments } from "./use-diff-comments";

const toastSuccess = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: { success: toastSuccess },
}));

describe("useDiffComments", () => {
  beforeEach(() => {
    sessionStorage.clear();
    toastSuccess.mockReset();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("stores an added comment without emitting a redundant success toast", () => {
    const { result } = renderHook(() =>
      useDiffComments({
        sessionId: "session-1",
        filePath: "src/example.ts",
        newContent: "first\nsecond\n",
      }),
    );

    act(() => {
      result.current.addComment({ start: 2, end: 2, side: "additions" }, "Tighten this");
    });

    expect({
      comments: useCommentsStore
        .getState()
        .getCommentsForFile("session-1", "src/example.ts")
        .map((comment) => comment.text),
      successToastCalls: toastSuccess.mock.calls.length,
    }).toEqual({
      comments: ["Tighten this"],
      successToastCalls: 0,
    });
  });
});
