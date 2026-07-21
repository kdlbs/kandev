import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useCommentsStore, type AgentMessageComment } from "@/lib/state/slices/comments";
import { usePendingAgentMessageComments } from "./use-pending-comments";

function messageComment(id: string, sessionId: string): AgentMessageComment {
  return {
    id,
    sessionId,
    source: "agent-message",
    messageId: `reply-${id}`,
    selectedText: "selected reply",
    anchor: {
      messageId: `reply-${id}`,
      start: 0,
      end: 14,
      selectedText: "selected reply",
      prefix: "",
      suffix: "",
    },
    text: "Please clarify this.",
    createdAt: "2026-07-20T00:00:00Z",
    status: "pending",
  };
}

describe("usePendingAgentMessageComments", () => {
  beforeEach(() => {
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("returns only comments for resolved session", () => {
    act(() => {
      useCommentsStore.getState().addComment(messageComment("one", "session-1"));
      useCommentsStore.getState().addComment(messageComment("two", "session-2"));
    });

    const { result } = renderHook(() => usePendingAgentMessageComments("session-1"));

    expect(result.current.map((comment) => comment.id)).toEqual(["one"]);
  });

  it("returns no comments when there is no resolved session", () => {
    act(() => {
      useCommentsStore.getState().addComment(messageComment("one", "session-1"));
    });

    const { result } = renderHook(() => usePendingAgentMessageComments(null));

    expect(result.current).toEqual([]);
  });
});
