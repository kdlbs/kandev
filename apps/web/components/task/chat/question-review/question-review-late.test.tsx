import { createRef } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationAnswer,
  type ClarificationQuestion,
  type Message,
} from "@/lib/types/http";
import { ClarificationInputOverlay } from "../clarification-input-overlay";
import { ClarificationDraftProvider } from "../clarification-draft-context";
import { clearAnswerDrafts } from "@/lib/clarification/answer-draft-cache";

const mocks = vi.hoisted(() => ({ fetchImage: vi.fn() }));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));
vi.mock("@/lib/api/domains/question-image-api", () => ({
  fetchQuestionImage: mocks.fetchImage,
  questionImageErrorCode: () => "unavailable",
}));
const mockStore = {
  getState: () => ({ updateMessage: vi.fn() }),
  subscribe: () => () => {},
};
const authState = { auth: { mode: "local", authenticated: true, user: { id: "u1" } } };
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof authState) => unknown) => selector(authState),
  useAppStoreApi: () => mockStore,
  useOptionalAppStoreApi: () => mockStore,
  useOptionalAppStore: (_selector: unknown, fallback: unknown) => fallback,
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const fetchMock = vi.fn();
const OPEN = "question-review-open";
const DIALOG = "question-review-dialog";
const SUBMIT = "question-review-submit";
const OPTION = "clarification-option";
const before = { id: "before", path: "before.png", alt: "Before" };
const after = { id: "after", path: "after.png", alt: "After" };

function expiredMessage(question: Partial<ClarificationQuestion>): Message {
  return {
    id: "m1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-11-24T00:00:00Z",
    metadata: {
      pending_id: "p1",
      question_id: "q1",
      question_index: 0,
      question_total: 1,
      status: "expired",
      question: {
        id: "q1",
        title: "",
        prompt: "Is the header right?",
        options: [
          { option_id: "o1", label: "Approve", description: "" },
          { option_id: "o2", label: "Revise", description: "" },
        ],
        ...question,
      },
    },
  };
}

type LateOutcome = "sent" | "queued";

function renderLateForm(
  messages: Message[],
  onLateAnswer: () => Promise<LateOutcome> = vi.fn().mockResolvedValue("sent"),
  initialAnswers?: readonly ClarificationAnswer[],
) {
  const scopeRef = createRef<HTMLDivElement>();
  const onDismiss = vi.fn();
  render(
    <ClarificationDraftProvider taskId="t1">
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={messages}
          mode="late"
          onLateAnswer={onLateAnswer}
          onResolved={vi.fn()}
          onDismiss={onDismiss}
          shortcutScopeRef={scopeRef}
          initialAnswers={initialAnswers}
        />
      </div>
    </ClarificationDraftProvider>,
  );
  return { onDismiss };
}

function respondCalls() {
  return fetchMock.mock.calls.filter(([url]) => String(url).includes("/respond"));
}

beforeEach(() => {
  fetchMock.mockReset();
  mocks.fetchImage.mockReset();
  mocks.fetchImage.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:test"), revokeObjectURL: vi.fn() }),
  );
});

afterEach(() => {
  cleanup();
  clearAnswerDrafts(mockStore);
});

describe("question image review in the late-answer form", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.1
  it("is offered for image bundles and not for image-free bundles", () => {
    renderLateForm([expiredMessage({ images: [before, after] })]);
    expect(screen.getByTestId(OPEN)).toBeTruthy();
    cleanup();
    renderLateForm([expiredMessage({})]);
    expect(screen.queryByTestId(OPEN)).toBeNull();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.3
  it("shares answer state and never sends when the view opens or closes", () => {
    const onLateAnswer = vi.fn().mockResolvedValue("sent");
    renderLateForm([expiredMessage({ images: [before, after] })], onLateAnswer);
    fireEvent.click(screen.getByTestId(OPEN));
    const dialog = screen.getByTestId(DIALOG);
    fireEvent.click(within(dialog).getAllByTestId(OPTION)[0]);
    fireEvent.click(within(dialog).getByTestId("question-review-close"));

    expect(screen.queryByTestId(DIALOG)).toBeNull();
    expect(screen.getAllByTestId(OPTION)[0].getAttribute("data-selected")).toBe("true");
    expect(onLateAnswer).not.toHaveBeenCalled();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.4
  it("sends from the view as a new message and closes on success", async () => {
    const onLateAnswer = vi.fn().mockResolvedValue("sent");
    renderLateForm([expiredMessage({ images: [before, after] })], onLateAnswer);
    fireEvent.click(screen.getByTestId(OPEN));
    const dialog = screen.getByTestId(DIALOG);
    const submit = within(dialog).getByTestId(SUBMIT) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(submit.textContent).toContain("Send as new message");

    fireEvent.click(within(dialog).getAllByTestId(OPTION)[0]);
    fireEvent.click(within(dialog).getByTestId(SUBMIT));

    await waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(1));
    expect(onLateAnswer.mock.calls[0][0].answers).toEqual([
      { question_id: "q1", selected_options: ["o1"] },
    ]);
    await waitFor(() => expect(screen.queryByTestId(DIALOG)).toBeNull());
    expect(screen.getByTestId("clarification-late-success")).toBeTruthy();
    expect(respondCalls()).toHaveLength(0);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.5
  it("stays open with the late error and keeps the answer when delivery fails", async () => {
    const onLateAnswer = vi.fn().mockRejectedValueOnce(new Error("offline"));
    renderLateForm([expiredMessage({ images: [before, after] })], onLateAnswer);
    fireEvent.click(screen.getByTestId(OPEN));
    const dialog = screen.getByTestId(DIALOG);
    fireEvent.click(within(dialog).getAllByTestId(OPTION)[0]);
    fireEvent.click(within(dialog).getByTestId(SUBMIT));

    await waitFor(() =>
      expect(within(dialog).getByTestId("clarification-submit-error")).toBeTruthy(),
    );
    expect(screen.getByTestId(DIALOG)).toBe(dialog);
    expect(within(dialog).getAllByTestId(OPTION)[0].getAttribute("data-selected")).toBe("true");

    onLateAnswer.mockResolvedValueOnce("queued");
    fireEvent.click(within(dialog).getByTestId("clarification-retry"));
    await waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByTestId(DIALOG)).toBeNull());
  });

  it("restores late-answer annotations on initial load or retry", async () => {
    const onLateAnswer = vi.fn().mockResolvedValue("sent");
    const testAnnotation = {
      image_id: "before",
      x: 10,
      y: 20,
      width: 30,
      height: 40,
      image_width: 100,
      image_height: 100,
      comment: "Fix this area",
    };
    renderLateForm([expiredMessage({ images: [before, after] })], onLateAnswer, [
      { question_id: "q1", selected_options: ["o1"], annotations: [testAnnotation] },
    ]);

    fireEvent.click(screen.getByTestId(OPEN));
    const dialog = screen.getByTestId(DIALOG);
    fireEvent.click(within(dialog).getByTestId(SUBMIT));

    await waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(1));
    expect(onLateAnswer.mock.calls[0][0].answers).toEqual([
      {
        question_id: "q1",
        selected_options: ["o1"],
        annotations: [testAnnotation],
      },
    ]);
  });
});
