import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  respond: vi.fn(),
  dismiss: vi.fn(),
  toast: vi.fn(),
  responsive: { isMobile: false, isFinePointer: true },
}));

vi.mock("@/lib/api/domains/agent-questions-api", () => ({
  respondToAgentQuestion: (...args: unknown[]) => mocks.respond(...args),
}));

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  dismissClarificationInboxBundle: (...args: unknown[]) => mocks.dismiss(...args),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: (...args: unknown[]) => mocks.toast(...args),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));

import { AsyncQuestionRow } from "./async-question-row";

const ROW_TOGGLE = "async-question-row-toggle";
const OPTION = "async-question-option";
const SUBMIT = "async-question-submit";
const CUSTOM_TEXT = "async-question-custom-text";
const ERROR = "async-question-error";
const ANSWER_SURFACE = "async-question-answer-surface";

function question(
  overrides: Partial<ClarificationInboxAsyncQuestion> = {},
): ClarificationInboxAsyncQuestion {
  return {
    question_id: "q1",
    task_id: "t1",
    session_id: "s1",
    session_state: "RUNNING",
    task_title: "Checkout integration",
    forwarded: false,
    waiting: false,
    status: "pending",
    context: "API integration",
    created_at: "2026-10-30T10:00:00Z",
    questions: [
      {
        id: "q1",
        title: "Database choice",
        prompt: "Which database should the service use?",
        options: [
          { option_id: "a", label: "Postgres", description: "Managed" },
          { option_id: "b", label: "SQLite", description: "Embedded" },
        ],
      },
    ],
    ...overrides,
  };
}

function renderRow(overrides: Partial<ClarificationInboxAsyncQuestion> = {}) {
  const onSettled = vi.fn();
  render(<AsyncQuestionRow question={question(overrides)} onSettled={onSettled} />);
  return { onSettled };
}

beforeEach(() => {
  mocks.responsive.isMobile = false;
  mocks.respond.mockResolvedValue({
    success: true,
    won: true,
    status: "answered",
    questions: [],
  });
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AsyncQuestionRow — presentation", () => {
  it("distinguishes a working agent from an explicitly waiting one", () => {
    renderRow();
    expect(screen.getByText("Agent continues work")).toBeTruthy();
    cleanup();
    renderRow({ waiting: true });
    expect(screen.getByText("Waiting for answer")).toBeTruthy();
  });

  it("dismisses with the namespaced async identity", async () => {
    mocks.dismiss.mockResolvedValue(undefined);
    const { onSettled } = renderRow();
    fireEvent.click(screen.getByTestId("async-question-dismiss"));
    await waitFor(() => expect(mocks.dismiss).toHaveBeenCalledWith("async:q1"));
    expect(onSettled).toHaveBeenCalled();
  });

  it("shows the forwarding parent for a forwarded child request", () => {
    renderRow({ forwarded: true, parent_task_title: "Planning" });
    expect(screen.getByTestId("async-question-provenance").textContent).toContain("Planning");
  });
});

describe("AsyncQuestionRow — answer flow", () => {
  it("submits the selected option and preserves the request identity", async () => {
    const { onSettled } = renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    fireEvent.click(screen.getAllByTestId(OPTION)[1]);
    fireEvent.click(screen.getByTestId(SUBMIT));
    await waitFor(() => expect(mocks.respond).toHaveBeenCalledTimes(1));
    const [questionId, body] = mocks.respond.mock.calls[0];
    expect(questionId).toBe("q1");
    expect(body.answers).toEqual([
      { question_id: "q1", selected_options: ["b"], custom_text: undefined },
    ]);
    expect(body.rejected).toBe(false);
    expect(body.submission_id).toBeTruthy();
    expect(onSettled).toHaveBeenCalled();
  });

  it("rejects the request as a rejection, not an affirmative answer", async () => {
    renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    fireEvent.click(screen.getByTestId("async-question-reject"));
    await waitFor(() => expect(mocks.respond).toHaveBeenCalledTimes(1));
    const [, body] = mocks.respond.mock.calls[0];
    expect(body.rejected).toBe(true);
    expect(body.answers).toEqual([]);
  });

  it("requires an option or text before submitting", async () => {
    renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    fireEvent.click(screen.getByTestId(SUBMIT));
    expect(await screen.findByTestId(ERROR)).toBeTruthy();
    expect(mocks.respond).not.toHaveBeenCalled();
  });

  it("retains a draft after a failed submission and reuses the submission id", async () => {
    mocks.respond.mockRejectedValueOnce(new Error("network"));
    renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    fireEvent.change(screen.getByTestId(CUSTOM_TEXT), {
      target: { value: "Keep the documented default" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT));
    expect(await screen.findByTestId(ERROR)).toBeTruthy();
    expect((screen.getByTestId(CUSTOM_TEXT) as HTMLTextAreaElement).value).toBe(
      "Keep the documented default",
    );
    mocks.respond.mockResolvedValueOnce({
      success: true,
      won: true,
      status: "answered",
      questions: [],
    });
    fireEvent.click(screen.getByTestId(SUBMIT));
    await waitFor(() => expect(mocks.respond).toHaveBeenCalledTimes(2));
    expect(mocks.respond.mock.calls[0][1].submission_id).toBe(
      mocks.respond.mock.calls[1][1].submission_id,
    );
  });

  it("opens a full-height answer surface on a phone", () => {
    mocks.responsive.isMobile = true;
    renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    expect(screen.getByTestId(ANSWER_SURFACE)).toBeTruthy();
    expect(screen.queryByTestId("async-question-inline-form")).toBeNull();
    fireEvent.click(screen.getByTestId("async-question-back"));
    expect(screen.queryByTestId(ANSWER_SURFACE)).toBeNull();
  });

  it("closes the phone surface on Escape", () => {
    mocks.responsive.isMobile = true;
    renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    expect(screen.getByTestId(ANSWER_SURFACE)).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByTestId(ANSWER_SURFACE)).toBeNull();
  });

  it("closes on another caller's winning answer and reports it", async () => {
    mocks.respond.mockResolvedValueOnce({
      success: true,
      won: false,
      status: "answered",
      questions: [],
    });
    const { onSettled } = renderRow();
    fireEvent.click(screen.getByTestId(ROW_TOGGLE));
    fireEvent.click(screen.getAllByTestId(OPTION)[0]);
    fireEvent.click(screen.getByTestId(SUBMIT));
    await waitFor(() => expect(mocks.toast).toHaveBeenCalled());
    expect(onSettled).toHaveBeenCalled();
  });

  it("shows withdrawn history with its reason", () => {
    renderRow({
      status: "withdrawn",
      withdraw_reason: "The documentation supplied the value.",
    });
    expect(screen.getByTestId("async-question-terminal-status").textContent).toBe("Withdrawn");
  });
});
