import type {
  DiffComment,
  PlanComment,
  WalkthroughComment,
  AgentMessageComment,
  ReviewFileComment,
} from "@/lib/state/slices/comments";

// i18n-exempt: Test-only feedback text, never displayed by the application.
export function makeDiffComment(text = "fix this"): DiffComment {
  return {
    id: "c-1",
    source: "diff",
    sessionId: "sess-1",
    filePath: "src/app.ts",
    startLine: 10,
    endLine: 12,
    side: "additions",
    codeContent: "const x = 1;",
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

// i18n-exempt: Test-only feedback text, never displayed by the application.
export function makePlanComment(text = "split step 2"): PlanComment {
  return {
    id: "c-2",
    source: "plan",
    sessionId: "",
    taskId: "task-1",
    planId: "plan-1",
    version: 2,
    text,
    selectedText: "step 2",
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

// i18n-exempt: Test-only feedback text, never displayed by the application.
export function makeWalkthroughComment(text = "explain this step"): WalkthroughComment {
  return {
    id: "c-3",
    source: "walkthrough",
    sessionId: "sess-1",
    taskId: "task-1",
    walkthroughId: "wt-1",
    walkthroughTitle: "Tour",
    stepIndex: 0,
    stepCount: 2,
    filePath: "src/app.ts",
    startLine: 10,
    endLine: 12,
    stepText: "Agent explanation",
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

// i18n-exempt: Test-only feedback text, never displayed by the application.
export function makeAgentMessageComment(text = "expand this answer"): AgentMessageComment {
  return {
    id: "c-4",
    source: "agent-message",
    sessionId: "sess-1",
    messageId: "reply-1",
    selectedText: "settled answer",
    anchor: {
      messageId: "reply-1",
      start: 0,
      end: 14,
      selectedText: "settled answer",
      prefix: "",
      suffix: "",
    },
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

export function makeReviewFileComment(): ReviewFileComment {
  return {
    id: "c-1",
    source: "review-file",
    sessionId: "sess-1",
    repositoryName: "api",
    filePath: "a.txt",
    text: "feedback",
    createdAt: "now",
    status: "pending",
  };
}
