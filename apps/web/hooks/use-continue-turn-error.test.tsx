import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { continueTurnErrorMessageID, useContinueTurnError } from "./use-continue-turn-error";

const sendMessageRequestMock = vi.hoisted(() => vi.fn());
const acknowledgementFailureMock = vi.hoisted(() => vi.fn());

vi.mock("./message-request", () => ({
  sendMessageRequest: sendMessageRequestMock,
}));
vi.mock("@/lib/task-turn-error-feedback", () => ({
  notifyTurnErrorAcknowledgementFailure: acknowledgementFailureMock,
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const STAMP = "failure-1";
const BASE_TIME = "2026-05-02T00:00:00Z";

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    state: "WAITING_FOR_INPUT",
    started_at: BASE_TIME,
    updated_at: BASE_TIME,
    ...overrides,
  };
}

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message-1",
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    author_type: "agent",
    content: "error",
    type: "message",
    created_at: BASE_TIME,
    ...overrides,
  };
}

function wrapper({
  sessionOverride = session(),
  messages = [],
  initialState = {},
}: {
  sessionOverride?: TaskSession | null;
  messages?: Message[];
  initialState?: Record<string, unknown>;
} = {}) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          taskSessions: {
            items: sessionOverride ? { [SESSION_ID]: sessionOverride } : {},
          },
          messages: { bySession: { [SESSION_ID]: messages }, metaBySession: {} },
          queue: {
            bySessionId: {},
            metaBySessionId: {},
            activeOperationBySessionId: {},
            nextOperationGeneration: 0,
          },
          ...initialState,
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

function renderContinue(
  options: Parameters<typeof wrapper>[0] = {},
  args: { taskId?: string; sessionId?: string; stamp?: string } = {},
) {
  return renderHook(
    () =>
      useContinueTurnError({
        taskId: args.taskId ?? TASK_ID,
        sessionId: args.sessionId ?? SESSION_ID,
        stamp: args.stamp ?? STAMP,
      }),
    { wrapper: wrapper(options) },
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useContinueTurnError readiness", () => {
  it("is ready for the current waiting session error", () => {
    const { result } = renderContinue();
    expect(result.current.ready).toBe(true);
    expect(result.current.messageId).toBe(continueTurnErrorMessageID(SESSION_ID, STAMP));
  });

  it.each(["RUNNING", "STARTING", "CANCELLED", "FAILED", "COMPLETED"] as const)(
    "is unavailable while the session is %s",
    (state) => {
      const { result } = renderContinue({ sessionOverride: session({ state }) });
      expect(result.current.ready).toBe(false);
    },
  );

  it("is unavailable with pending input, cancellation, or runtime recovery", () => {
    expect(
      renderContinue({ sessionOverride: session({ pending_action: "permission" }) }).result.current
        .ready,
    ).toBe(false);
    expect(
      renderContinue({ sessionOverride: session({ cancellation_pending: true }) }).result.current
        .ready,
    ).toBe(false);
    expect(
      renderContinue({
        sessionOverride: session({
          metadata: { last_agent_error: { message: "runtime died", occurred_at: BASE_TIME } },
        }),
      }).result.current.ready,
    ).toBe(false);
    expect(
      renderContinue({
        sessionOverride: session({
          session_recovery_blocks: [
            {
              id: "block-1",
              incarnation_id: "inc-1",
              expected_generation: 1,
              reason: "delivery",
              updated_at: BASE_TIME,
            },
          ],
        }),
      }).result.current.ready,
    ).toBe(false);
  });

  it("is unavailable during automatic retry waiting", () => {
    const { result } = renderContinue({
      messages: [message({ metadata: { retrying: true } })],
    });
    expect(result.current.ready).toBe(false);
  });

  it("is unavailable once this stamp's continuation was accepted", () => {
    const acceptedMessage = message({ id: continueTurnErrorMessageID(SESSION_ID, STAMP) });
    const { result } = renderContinue({ messages: [acceptedMessage] });
    expect(result.current.ready).toBe(false);
  });

  it("does not sample another session or an empty stamp", () => {
    expect(renderContinue({}, { sessionId: "other" }).result.current.ready).toBe(false);
    expect(renderContinue({}, { stamp: "" }).result.current.ready).toBe(false);
  });
});

describe("useContinueTurnError submission", () => {
  it("sends exact continue content with the observed stamp and no composer state", async () => {
    const created = message({
      id: continueTurnErrorMessageID(SESSION_ID, STAMP),
      author_type: "user",
      content: "continue",
    });
    sendMessageRequestMock.mockResolvedValue(created);
    const { result, rerender } = renderContinue();
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(sendMessageRequestMock).toHaveBeenCalledTimes(1);
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
    expect(result.current.outcome).toBe("accepted");
    expect(result.current.ready).toBe(false);
    rerender();
  });
});

describe("useContinueTurnError applied configuration", () => {
  it("uses the target session's applied plan mode without touching composer state", async () => {
    sendMessageRequestMock.mockResolvedValue(
      message({
        id: continueTurnErrorMessageID(SESSION_ID, STAMP),
        author_type: "user",
        content: "continue",
      }),
    );
    const { result } = renderContinue({
      sessionOverride: session({ metadata: { plan_mode: true } }),
    });
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(sendMessageRequestMock.mock.calls[0][0].planMode).toBe(true);
  });

  it("ignores an unapplied composer plan-mode toggle", async () => {
    sendMessageRequestMock.mockResolvedValue(
      message({
        id: continueTurnErrorMessageID(SESSION_ID, STAMP),
        author_type: "user",
        content: "continue",
      }),
    );
    const { result } = renderHook(
      () => ({
        hook: useContinueTurnError({
          taskId: TASK_ID,
          sessionId: SESSION_ID,
          stamp: STAMP,
        }),
        store: useAppStoreApi(),
      }),
      {
        wrapper: wrapper({
          initialState: {
            chatInput: {
              planModeBySessionId: { [SESSION_ID]: true },
              cancellingBySessionId: {},
            },
          },
        }),
      },
    );
    await act(async () => {
      await result.current.hook.continueTurn();
    });
    expect(sendMessageRequestMock.mock.calls[0][0].planMode).toBe(false);
    expect(result.current.store.getState().chatInput.planModeBySessionId[SESSION_ID]).toBe(true);
  });

  it("keeps the captured plan mode immutable across retries for the same error", async () => {
    sendMessageRequestMock.mockRejectedValueOnce(new Error("network"));
    sendMessageRequestMock.mockResolvedValueOnce(
      message({
        id: continueTurnErrorMessageID(SESSION_ID, STAMP),
        author_type: "user",
        content: "continue",
      }),
    );
    const { result } = renderHook(
      () => ({
        hook: useContinueTurnError({
          taskId: TASK_ID,
          sessionId: SESSION_ID,
          stamp: STAMP,
        }),
        store: useAppStoreApi(),
      }),
      {
        wrapper: wrapper({
          sessionOverride: session({ metadata: { plan_mode: true } }),
        }),
      },
    );
    await act(async () => {
      await result.current.hook.continueTurn();
    });
    expect(sendMessageRequestMock.mock.calls[0][0].planMode).toBe(true);

    act(() => {
      result.current.store.setState((state) => ({
        taskSessions: {
          ...state.taskSessions,
          items: {
            ...state.taskSessions.items,
            [SESSION_ID]: session({ metadata: { plan_mode: false } }),
          },
        },
      }));
    });
    await act(async () => {
      await result.current.hook.continueTurn();
    });
    expect(sendMessageRequestMock.mock.calls[1][0].planMode).toBe(true);
  });
});

describe("useContinueTurnError retry identity", () => {
  it("does not reopen continuation when acceptance returns no message", async () => {
    sendMessageRequestMock.mockResolvedValue(undefined);
    const { result } = renderContinue();
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(result.current.outcome).toBe("accepted");
    expect(result.current.available).toBe(false);
    expect(result.current.ready).toBe(false);
  });

  it("keeps one logical continuation for rapid activation", async () => {
    let resolveSend: (message: Message | undefined) => void = () => {};
    sendMessageRequestMock.mockImplementation(
      () => new Promise((resolve) => (resolveSend = resolve)),
    );
    const { result } = renderContinue();
    await act(async () => {
      const first = result.current.continueTurn();
      const second = result.current.continueTurn();
      resolveSend(undefined);
      await Promise.all([first, second]);
    });
    expect(sendMessageRequestMock).toHaveBeenCalledTimes(1);
  });

  it("keeps attention and localized failure state after a rejected send", async () => {
    sendMessageRequestMock.mockRejectedValue(new Error("network"));
    const { result } = renderContinue();
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(result.current.outcome).toBe("failed");
    expect(result.current.ready).toBe(true);
    expect(sendMessageRequestMock).toHaveBeenCalledTimes(1);
  });
});

describe("useContinueTurnError reconciliation", () => {
  it("reuses the same stable message id after a lost response", async () => {
    const stableID = continueTurnErrorMessageID(SESSION_ID, STAMP);
    sendMessageRequestMock.mockRejectedValueOnce(new Error("websocket request timed out"));
    sendMessageRequestMock.mockResolvedValueOnce(
      message({ id: stableID, author_type: "user", content: "continue" }),
    );
    const { result } = renderContinue();
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(result.current.outcome).toBe("failed");
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(result.current.outcome).toBe("accepted");
    expect(sendMessageRequestMock).toHaveBeenCalledTimes(2);
    expect(sendMessageRequestMock.mock.calls[0][0].clientMessageId).toBe(stableID);
    expect(sendMessageRequestMock.mock.calls[1][0].clientMessageId).toBe(stableID);
  });

  it("stays unavailable after a remount when the accepted message is present", () => {
    const accepted = message({
      id: continueTurnErrorMessageID(SESSION_ID, STAMP),
      author_type: "user",
      content: "continue",
    });
    const first = renderContinue({ messages: [accepted] });
    expect(first.result.current.ready).toBe(false);
    first.unmount();
    const second = renderContinue({ messages: [accepted] });
    expect(second.result.current.ready).toBe(false);
  });

  it("does not resend after acceptance when acknowledgement failed", async () => {
    sendMessageRequestMock.mockResolvedValue(
      message({
        id: continueTurnErrorMessageID(SESSION_ID, STAMP),
        author_type: "user",
        content: "continue",
        turn_error_acknowledgement_failed: true,
      }),
    );
    const { result } = renderContinue();
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(acknowledgementFailureMock).toHaveBeenCalledWith(true);
    expect(result.current.ready).toBe(false);
    await act(async () => {
      await result.current.continueTurn();
    });
    expect(sendMessageRequestMock).toHaveBeenCalledTimes(1);
  });
});

describe("continueTurnErrorMessageID", () => {
  it("is stable across calls and bounded", () => {
    const first = continueTurnErrorMessageID(SESSION_ID, STAMP);
    expect(continueTurnErrorMessageID(SESSION_ID, STAMP)).toBe(first);
    expect(first.length).toBeLessThanOrEqual(128);
    expect(continueTurnErrorMessageID(SESSION_ID, "x".repeat(400)).length).toBeLessThanOrEqual(128);
  });
});
