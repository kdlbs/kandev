import { describe, expect, it, vi } from "vitest";
import { buildContextItems } from "./chat-context-items";
import type { AgentMessageComment } from "@/lib/state/slices/comments";

const comment: AgentMessageComment = {
  id: "message-comment-1",
  sessionId: "session-1",
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
};

describe("buildContextItems agent-message comments", () => {
  it("creates one removable context item for pending message comments", () => {
    const items = buildContextItems({
      planContextEnabled: false,
      contextFiles: [],
      resolvedSessionId: "session-1",
      removeContextFile: vi.fn(),
      unpinFile: vi.fn(),
      addPlan: vi.fn(),
      promptsMap: new Map(),
      pendingCommentsByFile: {},
      handleRemoveCommentFile: vi.fn(),
      handleRemoveComment: vi.fn(),
      planComments: [],
      handleClearPlanComments: vi.fn(),
      pendingPRFeedback: [],
      handleRemovePRFeedback: vi.fn(),
      handleClearPRFeedback: vi.fn(),
      walkthroughComments: [],
      handleRemoveWalkthroughComment: vi.fn(),
      handleClearWalkthroughComments: vi.fn(),
      messageComments: [comment],
      handleClearMessageComments: vi.fn(),
      taskId: "task-1",
    } as never);

    const item = items.find((candidate) => candidate.kind === "agent-message-comment");
    expect(item).toMatchObject({
      kind: "agent-message-comment",
      label: "1 message comment",
    });
  });
});
