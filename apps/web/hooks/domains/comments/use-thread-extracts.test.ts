import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import { useThreadExtracts } from "./use-thread-extracts";

const extract = (authorName: string, quote: string, text: string): AgentMessageComment => ({
  id: quote,
  sessionId: "office-task:t",
  source: "agent-message",
  messageId: "c",
  authorName,
  selectedText: quote,
  anchor: {
    messageId: "c",
    start: 0,
    end: quote.length,
    selectedText: quote,
    prefix: "",
    suffix: "",
  },
  text,
  createdAt: "",
  status: "pending",
});

describe("useThreadExtracts", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("keeps an extract edit made while its submitted comment is pending", () => {
    const original = extract("CEO", "the plan", "Too vague.");
    act(() => useCommentsStore.getState().addComment(original));
    const { result } = renderHook(() => useThreadExtracts("t"));
    const submitted = result.current.extracts;

    act(() => useCommentsStore.getState().updateComment(original.id, { text: "Add a date." }));
    act(() => result.current.markSent(submitted));

    expect(useCommentsStore.getState().byId[original.id]).toMatchObject({
      text: "Add a date.",
      status: "pending",
    });
  });
});
