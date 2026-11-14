import { describe, expect, it } from "vitest";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import { filterVisibleMessages } from "./processed-message-filtering";

function transcriptMessage(
  id: string,
  author_type: Message["author_type"],
  content: string,
  metadata?: Record<string, unknown>,
  type: Message["type"] = "status",
): Message {
  return {
    id,
    session_id: sessionId("s1"),
    task_id: taskId("t1"),
    turn_id: id,
    author_type,
    content,
    type,
    metadata,
    created_at: "2026-05-30T00:00:00Z",
  };
}

describe("filterVisibleMessages passthrough mode", () => {
  it("hides only synthetic empty-turn notices and retains stored text and errors", () => {
    const messages = [
      transcriptMessage("empty-turn", "agent", "No chat output.", { empty_turn: true }),
      transcriptMessage("prompt", "user", "Please inspect the project.", undefined, "message"),
      transcriptMessage("reply", "agent", "I found an issue.", undefined, "message"),
      transcriptMessage("real-error", "agent", "Command failed.", { error_terminated: true }),
    ];

    expect(
      filterVisibleMessages(messages, new Set(), new Set(), undefined, true).map(
        (message) => message.id,
      ),
    ).toEqual(["prompt", "reply", "real-error"]);
    expect(
      filterVisibleMessages(messages, new Set(), new Set(), undefined, false).map(
        (message) => message.id,
      ),
    ).toEqual(["empty-turn", "prompt", "reply", "real-error"]);
  });
});
