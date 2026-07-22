import { beforeEach, describe, expect, it } from "vitest";
import { useCommentsStore } from "./comments-store";
import { COMMENTS_STORAGE_PREFIX, loadSessionComments } from "./persistence";
import type { AgentMessageComment } from "./types";

const SESSION_ID = "session-message-comments";

function messageComment(overrides: Partial<AgentMessageComment> = {}): AgentMessageComment {
  return {
    id: "message-comment-1",
    sessionId: SESSION_ID,
    source: "agent-message",
    messageId: "reply-1",
    selectedText: "selected answer",
    anchor: {
      messageId: "reply-1",
      start: 0,
      end: 15,
      selectedText: "selected answer",
      prefix: "",
      suffix: " continues",
    },
    text: "Please clarify this.",
    createdAt: "2026-07-20T00:00:00Z",
    status: "pending",
    ...overrides,
  };
}

describe("agent-message comment persistence", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("persists pending message comments in sessionStorage", () => {
    const comment = messageComment();

    useCommentsStore.getState().addComment(comment);

    expect(loadSessionComments(SESSION_ID)).toEqual([comment]);
    expect(window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`)).not.toBeNull();
  });

  it("hydrates message comments and pending context after a remount", () => {
    const comment = messageComment();
    window.sessionStorage.setItem(
      `${COMMENTS_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify([comment]),
    );

    useCommentsStore.getState().hydrateSession(SESSION_ID);

    const state = useCommentsStore.getState();
    expect(state.byId[comment.id]).toEqual(comment);
    expect(state.bySession[SESSION_ID]).toEqual([comment.id]);
    expect(state.pendingForChat).toEqual([comment.id]);
    expect(state.getPendingComments()).toEqual([comment]);
  });
});
