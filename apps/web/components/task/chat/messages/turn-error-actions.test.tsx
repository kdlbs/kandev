import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { TurnErrorActions } from "./turn-error-actions";

const sendMessageRequestMock = vi.hoisted(() => vi.fn());
const acknowledgementFailureMock = vi.hoisted(() => vi.fn());
const dismissTurnErrorMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/message-request", () => ({
  sendMessageRequest: sendMessageRequestMock,
}));
vi.mock("@/lib/task-turn-error-feedback", () => ({
  notifyTurnErrorAcknowledgementFailure: acknowledgementFailureMock,
}));
vi.mock("@/lib/api/domains/task-turn-error-api", () => ({
  dismissTaskTurnError: dismissTurnErrorMock,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const TASK_ID = "task-1";
const CONTINUE_TEST_ID = "turn-error-continue-button";
const DISMISS_TEST_ID = "turn-error-dismiss-button";

const SESSION_ID = "session-1";
const STAMP = "failure-1";

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-05-02T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function wrapper({ sessionOverride = session(), messages = [] as Message[] } = {}) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          taskSessions: { items: { [SESSION_ID]: sessionOverride } },
          messages: { bySession: { [SESSION_ID]: messages }, metaBySession: {} },
          queue: {
            bySessionId: {},
            metaBySessionId: {},
            activeOperationBySessionId: {},
            nextOperationGeneration: 0,
          },
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

function renderActions(options: Parameters<typeof wrapper>[0] = {}) {
  return render(<TurnErrorActions taskId={TASK_ID} sessionId={SESSION_ID} stamp={STAMP} />, {
    wrapper: wrapper(options),
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  dismissTurnErrorMock.mockResolvedValue({ dismissed: true });
});

describe("TurnErrorActions", () => {
  it("renders Dismiss and continue first, then Dismiss turn error", () => {
    renderActions();
    const group = screen.getByTestId("turn-error-actions");
    const buttons = Array.from(group.querySelectorAll("button"));
    expect(buttons.map((button) => button.dataset.testid)).toEqual([
      CONTINUE_TEST_ID,
      DISMISS_TEST_ID,
    ]);
    expect(buttons[0]?.textContent).toContain("Dismiss and continue");
    expect(buttons[1]?.textContent).toContain("Dismiss turn error");
  });

  it("disables both competing controls and announces one Continuing status", async () => {
    let resolveSend: (value: unknown) => void = () => {};
    sendMessageRequestMock.mockImplementation(
      () => new Promise((resolve) => (resolveSend = resolve)),
    );
    renderActions();
    await act(async () => {
      fireEvent.click(screen.getByTestId(CONTINUE_TEST_ID));
    });
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Continuing"));
    expect((screen.getByTestId(CONTINUE_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId(DISMISS_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      resolveSend(undefined);
    });
  });

  it("keeps both actions available with localized failure feedback after a rejected send", async () => {
    sendMessageRequestMock.mockRejectedValue(new Error("network"));
    renderActions();
    await act(async () => {
      fireEvent.click(screen.getByTestId(CONTINUE_TEST_ID));
    });
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toContain("Could not continue the turn"),
    );
    expect((screen.getByTestId(CONTINUE_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByTestId(DISMISS_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
  });

  it("disables continuation while dismissal is in flight", async () => {
    let resolveDismiss: (value: unknown) => void = () => {};
    dismissTurnErrorMock.mockImplementation(
      () => new Promise((resolve) => (resolveDismiss = resolve)),
    );
    renderActions();
    await act(async () => {
      fireEvent.click(screen.getByTestId(DISMISS_TEST_ID));
    });
    await waitFor(() =>
      expect((screen.getByTestId(CONTINUE_TEST_ID) as HTMLButtonElement).disabled).toBe(true),
    );
    expect((screen.getByTestId(DISMISS_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      resolveDismiss({ dismissed: true });
    });
  });

  it("does not offer continuation during automatic retry waiting", () => {
    renderActions({
      messages: [
        {
          id: "retry-1",
          session_id: toSessionId(SESSION_ID),
          metadata: { retrying: true },
        } as unknown as Message,
      ],
    });
    expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull();
    expect(screen.getByTestId(DISMISS_TEST_ID)).toBeTruthy();
  });

  it("does not offer continuation while another foreground turn owns the session", () => {
    renderActions({ sessionOverride: session({ state: "RUNNING" }) });
    expect(screen.queryByTestId(CONTINUE_TEST_ID)).toBeNull();
  });
});
