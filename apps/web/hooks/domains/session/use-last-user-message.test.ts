import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import type { Message } from "@/lib/types/http";

const mockListTaskSessionMessages = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessionMessages: (...args: unknown[]) => mockListTaskSessionMessages(...args),
}));

vi.mock("@/components/task/chat/message-list-shared", () => ({
  getLastUserMessageId: (messages: Message[]) => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].author_type === "user") return messages[i].id;
    }
    return null;
  },
}));

import { useLastUserMessage } from "./use-last-user-message";

function message(id: string, authorType: Message["author_type"]): Message {
  return { id, author_type: authorType, content: id, created_at: "", type: "message" } as Message;
}

const userPrompt = message("user-1", "user");
const agentReply = message("agent-1", "agent");

describe("useLastUserMessage", () => {
  beforeEach(() => {
    mockListTaskSessionMessages.mockReset();
  });

  it("returns the window-derived last user message without fetching", () => {
    const messages = [userPrompt, agentReply, message("agent-2", "agent")];
    const { result } = renderHook(() => useLastUserMessage("session-1", messages));

    expect(result.current.lastPromptMessage).toEqual(userPrompt);
    expect(mockListTaskSessionMessages).not.toHaveBeenCalled();
  });

  it("fetches the last user message when the window has none", async () => {
    mockListTaskSessionMessages.mockResolvedValue({ messages: [userPrompt], has_more: false });
    const messages = [agentReply, message("agent-2", "agent")];

    const { result } = renderHook(() => useLastUserMessage("session-1", messages));

    await waitFor(() => expect(result.current.lastPromptMessage).toEqual(userPrompt));
    expect(mockListTaskSessionMessages).toHaveBeenCalledWith("session-1", {
      limit: 1,
      author_type: "user",
      sort: "desc",
    });
  });

  it("returns null when the fetch finds no user message", async () => {
    mockListTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    const messages = [agentReply];

    const { result } = renderHook(() => useLastUserMessage("session-1", messages));

    await waitFor(() => expect(result.current.lastPromptMessage).toBeNull());
  });

  it("prefers a newly-loaded window message over the fetched fallback", async () => {
    mockListTaskSessionMessages.mockResolvedValue({ messages: [userPrompt], has_more: false });
    const windowOnly = [agentReply];

    const { result, rerender } = renderHook(
      ({ sessionId, messages }) => useLastUserMessage(sessionId, messages),
      {
        initialProps: { sessionId: "session-1", messages: windowOnly },
      },
    );
    await waitFor(() => expect(result.current.lastPromptMessage).toEqual(userPrompt));

    // A user message now arrives in the window (e.g. the user sends a new prompt).
    rerender({ sessionId: "session-1", messages: [userPrompt, agentReply] });
    expect(result.current.lastPromptMessage).toEqual(userPrompt);
    // The fetched fallback no longer drives the result; window message wins.
    mockListTaskSessionMessages.mockResolvedValue({ messages: [userPrompt], has_more: false });
    await waitFor(() => expect(result.current.lastPromptMessage).toEqual(userPrompt));
  });

  it("does not refetch while the session is unchanged", async () => {
    mockListTaskSessionMessages.mockResolvedValue({ messages: [userPrompt], has_more: false });
    const messages = [agentReply];

    const { rerender } = renderHook(() => useLastUserMessage("session-1", messages));
    await waitFor(() => expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(1));

    rerender();
    await waitFor(() => expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(1));
  });
});
