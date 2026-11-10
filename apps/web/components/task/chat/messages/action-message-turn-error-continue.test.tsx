import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { continueTurnErrorMessageID } from "@/hooks/use-continue-turn-error";
import { ActionMessage } from "./action-message";

const sendMessageRequestMock = vi.hoisted(() => vi.fn());
const acknowledgementFailureMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/message-request", () => ({
  sendMessageRequest: sendMessageRequestMock,
}));
vi.mock("@/lib/task-turn-error-feedback", () => ({
  notifyTurnErrorAcknowledgementFailure: acknowledgementFailureMock,
}));
vi.mock("@/lib/api/domains/task-turn-error-api", () => ({
  dismissTaskTurnError: vi.fn().mockResolvedValue({ dismissed: true }),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const TASK_ID = "task-1";
const CONTINUE_TEST_ID = "turn-error-continue-button";
const DISMISS_TEST_ID = "turn-error-dismiss-button";

const SESSION_ID = "session-1";
const STAMP = "failure-current";
const TIME = "2026-05-02T00:00:00Z";

function errorMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: STAMP,
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    author_type: "agent",
    type: "error",
    content: "Selected model is at capacity. Please try a different model.",
    created_at: TIME,
    metadata: {
      variant: "error",
      failure_scope: "turn",
      runtime_retained: true,
      recovery_actions: false,
      recovery_disposition: "refused",
    },
    ...overrides,
  };
}

function turnError(message: Message, overrides: Partial<TaskStatusSummary> = {}) {
  return {
    turn_error: {
      session_id: SESSION_ID,
      message_id: message.id,
      stamp: STAMP,
      occurred_at: TIME,
      preview: "Selected model is at capacity.",
    },
    ...overrides,
  } as unknown as TaskStatusSummary;
}

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    state: "WAITING_FOR_INPUT",
    started_at: TIME,
    updated_at: TIME,
    ...overrides,
  };
}

function renderMessage(
  message: Message,
  {
    sessionOverride = session(),
    summary = turnError(message),
    messages = [] as Message[],
  }: {
    sessionOverride?: TaskSession;
    summary?: TaskStatusSummary;
    messages?: Message[];
  } = {},
) {
  const initialState = {
    taskSessions: { items: { [SESSION_ID]: sessionOverride } },
    messages: { bySession: { [SESSION_ID]: messages }, metaBySession: {} },
    queue: {
      bySessionId: {},
      metaBySessionId: {},
      activeOperationBySessionId: {},
      nextOperationGeneration: 0,
    },
    kanban: { workflowId: null, steps: [], tasks: [{ id: TASK_ID, statusSummary: summary }] },
  } as unknown as ComponentProps<typeof StateProvider>["initialState"];
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider initialState={initialState}>{children}</StateProvider>
  );
  return render(<ActionMessage comment={message} />, { wrapper });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ActionMessage Dismiss and continue", () => {
  it("offers continuation before dismissal for the current same-session error", () => {
    renderMessage(errorMessage());
    const actionGroup = screen.getByTestId("turn-error-actions");
    const buttons = Array.from(actionGroup.querySelectorAll("button"));
    expect(buttons.map((button) => button.dataset.testid)).toEqual([
      CONTINUE_TEST_ID,
      DISMISS_TEST_ID,
    ]);
  });

  it("submits exact continue content, acknowledges the observed stamp, and retires the action", async () => {
    const message = errorMessage();
    const accepted = {
      ...message,
      id: continueTurnErrorMessageID(SESSION_ID, STAMP),
      author_type: "user",
      content: "continue",
    } as Message;
    sendMessageRequestMock.mockResolvedValue(accepted);
    renderMessage(message);
    await act(async () => {
      fireEvent.click(screen.getByTestId(CONTINUE_TEST_ID));
    });
    expect(sendMessageRequestMock).toHaveBeenCalledWith({
      taskId: TASK_ID,
      resolvedSessionId: SESSION_ID,
      clientMessageId: continueTurnErrorMessageID(SESSION_ID, STAMP),
      finalMessage: "continue",
      modelToSend: undefined,
      planMode: false,
      observedTurnErrorStamp: STAMP,
    });
    expect(acknowledgementFailureMock).toHaveBeenCalledWith(undefined);
    await waitFor(() => expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull());
    expect(screen.getByTestId(DISMISS_TEST_ID)).toBeTruthy();
  });

  it("does not offer continuation for a historical error", () => {
    const historical = errorMessage({ id: "failure-old" });
    renderMessage(historical, {
      summary: turnError(errorMessage(), { turn_error: undefined }),
    });
    expect(screen.queryByTestId("turn-error-actions")).toBeNull();
    expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull();
  });

  it("does not offer continuation for another session's attention", () => {
    const message = errorMessage();
    renderMessage(message, {
      summary: {
        turn_error: {
          session_id: "sibling-session",
          message_id: message.id,
          stamp: STAMP,
          occurred_at: TIME,
          preview: "Sibling failed",
        },
      } as unknown as TaskStatusSummary,
    });
    expect(screen.queryByTestId("turn-error-actions")).toBeNull();
  });

  it("keeps dismissal but removes continuation while another foreground turn runs", () => {
    renderMessage(errorMessage(), { sessionOverride: session({ state: "RUNNING" }) });
    expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull();
    expect(screen.getByTestId(DISMISS_TEST_ID)).toBeTruthy();
  });

  it("keeps dismissal but removes continuation during automatic retry waiting", () => {
    const message = errorMessage();
    renderMessage(message, {
      messages: [
        {
          ...message,
          id: "retry-notice",
          session_id: toSessionId(SESSION_ID),
          type: "status",
          metadata: { retrying: true },
        } as Message,
      ],
    });
    expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull();
    expect(screen.getByTestId(DISMISS_TEST_ID)).toBeTruthy();
  });
});
