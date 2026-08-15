import { describe, expect, it } from "vitest";
import { buildSubmitMessage } from "./chat-input-area";
import { resolveStatusRowTaskId, shouldRenderChatStatusBar } from "./chat-status-bar";
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

describe("shouldRenderChatStatusBar", () => {
  it("removes empty taskless status chrome when its auto-scroll control is hidden", () => {
    expect(
      shouldRenderChatStatusBar({
        hasTask: false,
        hasTodos: false,
        hasQueueChip: false,
        showRightControls: false,
        showProceed: false,
      }),
    ).toBe(false);
  });
});

describe("resolveStatusRowTaskId", () => {
  it("prefers the session-derived task so existing hosts are unchanged", () => {
    expect(resolveStatusRowTaskId("from-session", "from-host")).toBe("from-session");
  });

  // The regression: a blocked chain step has no session, so the session-derived
  // id is null and the status row (and with it the dependency chip) disappeared
  // on exactly the tasks the chip exists to describe.
  it("falls back to the host's task when the task has no session yet", () => {
    expect(resolveStatusRowTaskId(null, "from-host")).toBe("from-host");
  });

  it("stays null when neither side knows a task", () => {
    expect(resolveStatusRowTaskId(null, null)).toBeNull();
  });
});
