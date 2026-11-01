import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  state: {
    questions: [] as ClarificationInboxAsyncQuestion[],
    status: "ready" as "idle" | "loading" | "ready" | "error",
    refresh: vi.fn(),
  },
  responsive: { isMobile: false, isFinePointer: true },
}));

vi.mock("@/hooks/domains/session/use-task-async-questions", () => ({
  useTaskAsyncQuestions: () => mocks.state,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));

import { AsyncQuestionStrip } from "./async-question-strip";

function question(overrides: Partial<ClarificationInboxAsyncQuestion> = {}) {
  return {
    question_id: "question-a",
    task_id: "task-a",
    session_id: "session-a",
    session_state: "RUNNING",
    task_title: "Checkout integration",
    forwarded: false,
    waiting: false,
    status: "pending" as const,
    context: "API integration",
    created_at: "2026-10-30T10:00:00Z",
    questions: [
      {
        id: "choice-a",
        title: "Database choice",
        prompt: "Which database should the service use?",
        options: [
          { option_id: "postgres", label: "Postgres", description: "Managed" },
          { option_id: "sqlite", label: "SQLite", description: "Embedded" },
        ],
      },
    ],
    ...overrides,
  } satisfies ClarificationInboxAsyncQuestion;
}

function renderStrip(hasBlockingClarification = false) {
  return render(
    <AsyncQuestionStrip
      workspaceId="workspace-a"
      taskId="task-a"
      hasBlockingClarification={hasBlockingClarification}
    />,
  );
}

beforeEach(() => {
  mocks.state.questions = [question(), question({ question_id: "question-b" })];
  mocks.state.status = "ready";
  mocks.state.refresh.mockReset();
  mocks.responsive.isMobile = false;
});

afterEach(cleanup);

describe("AsyncQuestionStrip", () => {
  it("shows the count above the composer and hides answer rows until opened", () => {
    renderStrip();
    expect(screen.getByText("2 open questions, agent continues work")).toBeTruthy();
    expect(screen.queryByTestId("task-async-question-list")).toBeNull();

    fireEvent.click(screen.getByTestId("task-async-question-toggle"));
    expect(screen.getAllByTestId("async-question-row")).toHaveLength(2);
    expect(screen.queryByTestId("async-question-dismiss")).toBeNull();
  });

  it("stays collapsed and keeps the count visible during blocking clarification", () => {
    renderStrip(true);
    expect(screen.getByText("2 open questions, agent continues work")).toBeTruthy();
    expect(screen.getByTestId("task-async-question-toggle")).toHaveProperty("disabled", true);
    expect(screen.queryByTestId("task-async-question-list")).toBeNull();
  });

  it("opens the shared full-height answer surface on a phone", () => {
    mocks.responsive.isMobile = true;
    renderStrip();
    fireEvent.click(screen.getByTestId("task-async-question-toggle"));
    fireEvent.click(screen.getAllByTestId("async-question-row-toggle")[0]);
    expect(screen.getByTestId("async-question-answer-surface")).toBeTruthy();
    expect(screen.queryByTestId("async-question-dismiss")).toBeNull();
  });

  it("shows a retry after a failed task read", () => {
    mocks.state.status = "error";
    mocks.state.questions = [];
    renderStrip();
    expect(screen.getByTestId("task-async-question-error")).toBeTruthy();
    fireEvent.click(screen.getByText("Retry"));
    expect(mocks.state.refresh).toHaveBeenCalledTimes(1);
  });
});
