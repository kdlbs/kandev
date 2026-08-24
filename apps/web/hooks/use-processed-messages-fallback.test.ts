import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import { useProcessedMessages } from "./use-processed-messages";

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
      useProcessedMessages([agentMessage], "t1", "s1", "task description"),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual([
      "task-description",
      "agent-1",
    ]);
  });

  it("does not duplicate a visible stored user prompt", () => {
    const userMessage = makeMessage("user-1", "user", "stored prompt");
    const { result } = renderHook(() =>
      useProcessedMessages([userMessage], "t1", "s1", "task description"),
    );

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["user-1"]);
  });

  it("does not create a fallback for an empty task description", () => {
    const agentMessage = makeMessage("agent-1", "agent", "boot");
    const { result } = renderHook(() => useProcessedMessages([agentMessage], "t1", "s1", ""));

    expect(result.current.allMessages.map((message) => message.id)).toEqual(["agent-1"]);
  });
});
