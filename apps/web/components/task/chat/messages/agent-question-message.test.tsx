import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import { sessionId, taskId, type Message } from "@/lib/types/http";

const detail = vi.hoisted(() => ({
  question: null as ClarificationInboxAsyncQuestion | null,
  status: "ready" as "loading" | "ready" | "error",
  refresh: vi.fn(),
}));

vi.mock("@/hooks/domains/session/use-agent-question-detail", () => ({
  useAgentQuestionDetail: () => detail,
}));

import { AgentQuestionMessage } from "./agent-question-message";

function message(): Message {
  return {
    id: "transcript-question-1",
    session_id: sessionId("session-1"),
    task_id: taskId("task-1"),
    author_type: "agent",
    content: "stored request",
    type: "agent_question_request",
    metadata: { async_question_id: "question-1" },
    created_at: "2026-10-30T10:00:00Z",
  };
}

function answeredQuestion(): ClarificationInboxAsyncQuestion {
  return {
    question_id: "question-1",
    task_id: "task-1",
    session_id: "session-1",
    session_state: "RUNNING",
    task_title: "Task",
    forwarded: false,
    waiting: false,
    status: "answered",
    context: "",
    created_at: "2026-10-30T10:00:00Z",
    questions: [
      {
        id: "database",
        title: "Database choice",
        prompt: "Which database should we use?",
        options: [
          { option_id: "postgres", label: "PostgreSQL", description: "Managed" },
          { option_id: "sqlite", label: "SQLite", description: "Embedded" },
        ],
      },
    ],
    outcome: {
      answers: [{ question_id: "database", selected_options: ["postgres"] }],
    },
  };
}

afterEach(() => {
  cleanup();
  detail.question = null;
  detail.status = "ready";
  detail.refresh.mockReset();
});

describe("AgentQuestionMessage", () => {
  it("renders the current outcome with the selected option label", () => {
    detail.question = answeredQuestion();
    render(<AgentQuestionMessage comment={message()} />);

    expect(screen.getByText("Question from agent")).toBeTruthy();
    expect(screen.getByText("Answered")).toBeTruthy();
    expect(screen.getByText("Database choice")).toBeTruthy();
    expect(screen.getByText("PostgreSQL")).toBeTruthy();
    expect(screen.queryByText("postgres")).toBeNull();
  });

  it("offers retry when the current state cannot be read", () => {
    detail.status = "error";
    render(<AgentQuestionMessage comment={message()} />);
    expect(screen.getByText("Question details are unavailable.")).toBeTruthy();
  });
});
