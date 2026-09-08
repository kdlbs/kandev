import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import { useProcessedMessages } from "./use-processed-messages";

const TASK_DESCRIPTION = "task description";

function makeMessage(id: string, authorType: "agent" | "user", content: string): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: authorType,
    content,
    type: "message" satisfies MessageType,
    created_at: "",
  };
}

describe("useProcessedMessages task description fallback", () => {
  it("keeps the task description before visible agent history", () => {
    const agentMessage = makeMessage("agent-1", "agent", "boot");
    const { result } = renderHook(() =>
      useProcessedMessages([agentMessage], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: true,
        hasOlderMessages: false,
      }),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual([
      "task-description",
      "agent-1",
    ]);
  });

  it("does not synthesize a fallback while older history remains", () => {
    const agentMessage = makeMessage("agent-1", "agent", "boot");
    const { result } = renderHook(() =>
      useProcessedMessages([agentMessage], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: true,
        hasOlderMessages: true,
      }),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["agent-1"]);
  });

  it("does not synthesize a fallback before history initializes", () => {
    const agentMessage = makeMessage("agent-1", "agent", "boot");
    const { result } = renderHook(() =>
      useProcessedMessages([agentMessage], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: false,
        hasOlderMessages: false,
      }),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["agent-1"]);
  });

  it("does not duplicate a visible stored user prompt", () => {
    const userMessage = makeMessage("user-1", "user", "stored prompt");
    const { result } = renderHook(() =>
      useProcessedMessages([userMessage], "t1", "s1", TASK_DESCRIPTION, {
        historyInitialized: true,
        hasOlderMessages: false,
      }),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["user-1"]);
  });

  it("does not create a fallback for an empty task description", () => {
    const agentMessage = makeMessage("agent-1", "agent", "boot");
    const { result } = renderHook(() => useProcessedMessages([agentMessage], "t1", "s1", ""));

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["agent-1"]);
  });

  it("emits the latest processed-message debug sample at most once per 250 ms", () => {
    vi.useFakeTimers();
    const debugSpy = vi.spyOn(console, "debug").mockImplementation(() => undefined);

    try {
      const initialMessage = makeMessage("agent-1", "agent", "boot");
      const { rerender } = renderHook(
        ({ messages }: { messages: Message[] }) => useProcessedMessages(messages, "t1", "s1", ""),
        { initialProps: { messages: [initialMessage] } },
      );

      rerender({
        messages: [
          initialMessage,
          makeMessage("agent-2", "agent", "second"),
          makeMessage("agent-3", "agent", "latest"),
        ],
      });

      expect(debugSpy).not.toHaveBeenCalled();
      vi.advanceTimersByTime(249);
      expect(debugSpy).not.toHaveBeenCalled();

      vi.advanceTimersByTime(1);
      expect(debugSpy).toHaveBeenCalledTimes(1);
      expect(debugSpy.mock.calls[0]?.[0]).toContain('input={"count":3');
    } finally {
      debugSpy.mockRestore();
      vi.useRealTimers();
    }
  });

  it("flushes pending debug samples on session changes and unmount", () => {
    vi.useFakeTimers();
    const debugSpy = vi.spyOn(console, "debug").mockImplementation(() => undefined);
    const firstMessage = makeMessage("agent-1", "agent", "first");
    const secondMessage = makeMessage("agent-2", "agent", "second");
    const { rerender, unmount } = renderHook(
      ({ sessionId, messages }: { sessionId: string; messages: Message[] }) =>
        useProcessedMessages(messages, "t1", sessionId, ""),
      { initialProps: { sessionId: "s1", messages: [firstMessage] } },
    );

    try {
      rerender({ sessionId: "s2", messages: [firstMessage, secondMessage] });

      expect(debugSpy).toHaveBeenCalledTimes(1);
      expect(debugSpy.mock.calls[0]?.[0]).toContain('input={"count":1');

      unmount();

      expect(debugSpy).toHaveBeenCalledTimes(2);
      expect(debugSpy.mock.calls[1]?.[0]).toContain('input={"count":2');
    } finally {
      debugSpy.mockRestore();
      vi.useRealTimers();
    }
  });
});

afterEach(() => {
  vi.useRealTimers();
});
