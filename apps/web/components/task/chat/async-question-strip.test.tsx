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

const QUESTION_TOGGLE_TEST_ID = "task-async-question-toggle";

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
  it("renders nothing while an empty question read is idle or loading", () => {
    mocks.state.questions = [];
    mocks.state.status = "idle";
    const { rerender } = renderStrip();
    expect(screen.queryByTestId("task-async-question-strip")).toBeNull();
    expect(screen.queryByText("Loading questions")).toBeNull();

    mocks.state.status = "loading";
    rerender(
      <AsyncQuestionStrip
        workspaceId="workspace-a"
        taskId="task-a"
        hasBlockingClarification={false}
      />,
    );
    expect(screen.queryByTestId("task-async-question-strip")).toBeNull();
    expect(screen.queryByText("Loading questions")).toBeNull();
  });

  it("keeps the last good rows visible during refresh and beside a read error", () => {
    mocks.state.status = "loading";
    renderStrip();
    expect(screen.getByTestId("task-async-question-strip")).toBeTruthy();
    expect(screen.queryByTestId("task-async-question-list")).toBeNull();
    fireEvent.click(screen.getByTestId(QUESTION_TOGGLE_TEST_ID));
    expect(screen.getAllByTestId("async-question-row")).toHaveLength(2);

    mocks.state.status = "error";
    cleanup();
    renderStrip();
    expect(screen.getByTestId("task-async-question-error")).toBeTruthy();
    fireEvent.click(screen.getByTestId(QUESTION_TOGGLE_TEST_ID));
    expect(screen.getAllByTestId("async-question-row")).toHaveLength(2);
  });

  it("shows the count above the composer and hides answer rows until opened", () => {
    renderStrip();
    expect(screen.getByText("2 open questions, agent continues work")).toBeTruthy();
    expect(screen.queryByTestId("task-async-question-list")).toBeNull();

    fireEvent.click(screen.getByTestId(QUESTION_TOGGLE_TEST_ID));
    expect(screen.getAllByTestId("async-question-row")).toHaveLength(2);
    expect(screen.queryByTestId("async-question-dismiss")).toBeNull();
  });

  it("stays collapsed and keeps the count visible during blocking clarification", () => {
    renderStrip(true);
    expect(screen.getByText("2 open questions, agent continues work")).toBeTruthy();
    expect(screen.getByTestId(QUESTION_TOGGLE_TEST_ID)).toHaveProperty("disabled", true);
    expect(screen.queryByTestId("task-async-question-list")).toBeNull();
  });

  it("opens the shared full-height answer surface on a phone", () => {
    mocks.responsive.isMobile = true;
    renderStrip();
    fireEvent.click(screen.getByTestId(QUESTION_TOGGLE_TEST_ID));
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

  it("uses coarse-pointer touch targets on tablet widths", () => {
    renderStrip();
    const toggle = screen.getByTestId(QUESTION_TOGGLE_TEST_ID);
    expect(toggle.className).toContain("min-h-7");
    expect(toggle.className).toContain("max-md:min-h-11");
    expect(toggle.className).toContain("[@media(pointer:coarse)]:min-h-11");
  });

  it("shows the parent as provenance in the requesting child's chat", () => {
    mocks.state.questions = [
      question({
        recipient: "parent",
        forwarded: true,
        parent_task_title: "Planning",
      }),
    ];
    renderStrip();
    fireEvent.click(screen.getByTestId(QUESTION_TOGGLE_TEST_ID));
    expect(screen.getByTestId("async-question-provenance").textContent).toBe(
      "From parent Planning",
    );
  });
});
