import { describe, expect, it } from "vitest";
import { buildSubmitMessage } from "./chat-input-area";
import type { AgentMessageComment } from "@/lib/state/slices/comments";

const messageComment: AgentMessageComment = {
  id: "message-comment-1",
  sessionId: "session-1",
  source: "agent-message",
  messageId: "reply-1",
  selectedText: "answer",
  anchor: {
    messageId: "reply-1",
    start: 0,
    end: 6,
    selectedText: "answer",
    prefix: "",
    suffix: "",
  },
  text: "Please make this more precise.",
  createdAt: "2026-07-20T00:00:00Z",
  status: "pending",
};

describe("buildSubmitMessage agent message comments", () => {
  it("includes selected response context while preserving ordinary prose", () => {
    const result = buildSubmitMessage({
      message: "Continue from here.",
      pendingPRFeedback: [],
      planComments: [],
      messageComments: [messageComment],
    });

    expect(result).toContain("### Agent Message Comments");
    expect(result).toContain("> answer");
    expect(result).toContain("Continue from here.");
  });
});
