/* eslint-disable max-lines -- recovery action variants share one rendering harness. */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { StoreApi } from "zustand";
import { ActionMessage } from "./action-message";

const MANAGED_RUNTIME_RETRY_TEST_ID = "managed-runtime-npm-retry-button";
const HISTORICAL_FAILURE_STAMP = "failure-old";
const CURRENT_FAILURE_STAMP = "failure-current";
const NEW_FAILURE_STAMP = "failure-new";
const RECOVERY_RESOLVED_AT = "2026-09-29T10:05:00Z";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { SessionRecoveryProvider } from "../session-recovery-context";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const requestMock = vi.fn().mockResolvedValue({});
const getWebSocketClientMock = vi.fn<() => { request: typeof requestMock } | null>(() => ({
  request: requestMock,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  getWebSocketClientMock.mockReturnValue({ request: requestMock });
});

const CANCEL_TEST_ID = "recovery-cancel-retry-button";
const TECHNICAL_DETAILS = "Technical details";
const RECOVERY_MESSAGE = "Agent encountered an error";
const RESUME_LABEL = "Resume session";
const RESUME_TEST_ID = "recovery-resume-button";
const STALL_CANCEL_TEST_ID = "stall-cancel-turn-button";
const TEST_SESSION_ID = "sess-1";
const TEST_TASK_ID = "task-1";
const SESSION_RECOVER_METHOD = "session.recover";

/** Builds a system status Message describing a transient provider retry, with an optional Cancel action. */
function retryMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId(TEST_SESSION_ID),
    task_id: toTaskId(TEST_TASK_ID),
    author_type: "system",
    content: "Provider overloaded — retrying in 5s (attempt 1/5)",
    type: "status",
    created_at: "2026-05-30T00:00:00Z",
    metadata: {
      variant: "warning",
      retrying: true,
      attempt: 1,
      max_attempts: 5,
      retry_in_seconds: 5,
      session_id: TEST_SESSION_ID,
      task_id: TEST_TASK_ID,
      actions: [
        {
          type: "ws_request",
          label: "Cancel",
          icon: "x",
          test_id: CANCEL_TEST_ID,
          params: {
            method: SESSION_RECOVER_METHOD,
            payload: { task_id: TEST_TASK_ID, session_id: TEST_SESSION_ID, action: "cancel_retry" },
          },
        },
      ],
    },
    ...overrides,
  } as Message;
}

/** Builds the "warning" retrying metadata object for a given attempt and retry delay. */
function transientRetryMetadata(attempt: number, retryInSeconds: number) {
  return {
    variant: "warning",
    retrying: true,
    attempt,
    max_attempts: 5,
    retry_in_seconds: retryInSeconds,
    session_id: TEST_SESSION_ID,
    task_id: TEST_TASK_ID,
    actions: [],
  };
}

/** Builds an error-variant recovery Message with an optional Resume action carrying request params. */
function recoveryMessage(withParams = false): Message {
  return retryMessage({
    content: RECOVERY_MESSAGE,
    metadata: {
      variant: "error",
      recovery_actions: true,
      actions: [
        {
          type: "ws_request",
          label: RESUME_LABEL,
          test_id: RESUME_TEST_ID,
          ...(withParams
            ? {
                params: {
                  method: SESSION_RECOVER_METHOD,
                  payload: { task_id: TEST_TASK_ID, session_id: TEST_SESSION_ID },
                },
              }
            : {}),
        },
      ],
    },
  } as Partial<Message>);
}

/** Builds a running-stall notice Message for the given turn with a Cancel turn action. */
function stalledMessage(turnId = "turn-1"): Message {
  return retryMessage({
    turn_id: turnId,
    content: "Still waiting on Start dev server.",
    metadata: {
      action_visibility: "running",
      actions: [
        {
          type: "ws_request",
          label: "Cancel turn",
          test_id: STALL_CANCEL_TEST_ID,
          params: { method: "agent.cancel", payload: { session_id: TEST_SESSION_ID } },
        },
      ],
    },
  } as Partial<Message>);
}

/** ActionMessage reads session state from the store (keyed by comment.session_id),
 *  so seed it via the provider instead of passing a prop. */
function renderAction(
  comment: Message,
  sessionState?: TaskSessionState,
  sessionError?: string,
  activeTurnId?: string,
  sessionMetadata?: Record<string, unknown> | null,
) {
  const initialState: Partial<AppState> = sessionState
    ? {
        taskSessions: {
          items: {
            [TEST_SESSION_ID]: {
              state: sessionState,
              error_message: sessionError,
              metadata: sessionMetadata,
            } as TaskSession,
          },
        },
        turns: {
          bySession: {},
          activeBySession: activeTurnId ? { [TEST_SESSION_ID]: activeTurnId } : {},
          loadedBySession: {},
          reconcileEpochBySession: {},
          settledBoundaryBySession: {},
        },
      }
    : {};
  return render(<ActionMessage comment={comment} />, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
}

/** Like renderAction, but captures the store so the test can drive live session
 *  state transitions (STARTING/RUNNING → WAITING_FOR_INPUT) the way a real
 *  resume does over the WebSocket. */
function renderActionWithStore(
  comment: Message,
  sessionState: TaskSessionState,
  sessionError = "",
  activeTurnId?: string,
) {
  let store: StoreApi<AppState> | null = null;
  function CaptureStore() {
    store = useAppStoreApi();
    return null;
  }
  const initialState: Partial<AppState> = {
    taskSessions: {
      items: {
        [TEST_SESSION_ID]: { state: sessionState, error_message: sessionError } as TaskSession,
      },
    },
    turns: {
      bySession: {},
      activeBySession: activeTurnId ? { [TEST_SESSION_ID]: activeTurnId } : {},
      loadedBySession: {},
      reconcileEpochBySession: {},
      settledBoundaryBySession: {},
    },
  };
  const utils = render(
    <StateProvider initialState={initialState}>
      <CaptureStore />
      <ActionMessage comment={comment} />
    </StateProvider>,
  );
  const setSessionState = (next: TaskSessionState) =>
    act(() => {
      store?.getState().setTaskSession({
        id: toSessionId(TEST_SESSION_ID),
        task_id: toTaskId(TEST_TASK_ID),
        state: next,
        started_at: "",
        updated_at: "",
      } as TaskSession);
    });
  const addMessage = (message: Message) => act(() => store?.getState().addMessage(message));
  const updateMessage = (message: Message) =>
    act(() => store?.getState().updateMessages([message]));
  return { ...utils, setSessionState, addMessage, updateMessage };
}

describe("ActionMessage — transient retry (warning variant)", () => {
  it("renders the retrying copy in amber, not red", () => {
    renderAction(retryMessage(), "WAITING_FOR_INPUT");
    const text = screen.getByLabelText("Retry countdown");
    expect(text.textContent).toMatch(/retrying in 0:0[45]/i);
    expect(text.parentElement?.className).toContain("text-amber-600");
    expect(text.parentElement?.className).not.toContain("text-red-600");
  });

  it("renders a localized countdown from the persisted retry deadline", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-08T12:00:00.000Z"));
    try {
      renderAction(
        retryMessage({
          content: "Provider error",
          metadata: {
            variant: "warning",
            retrying: true,
            attempt: 1,
            max_attempts: 5,
            retry_at: "2026-08-08T12:01:05.000Z",
            failure_code: "model_capacity",
            provider_name: "Codex",
            model_id: "gpt-5",
            session_id: TEST_SESSION_ID,
            task_id: TEST_TASK_ID,
            actions: [],
          },
        }),
        "WAITING_FOR_INPUT",
      );
      expect(screen.getByTestId("transient-retry-card")).toBeTruthy();
      expect(screen.getByText(/retrying in 1:05/i)).toBeTruthy();
      expect(screen.getByText(/Codex · gpt-5/i)).toBeTruthy();
      expect(screen.getByText(/attempt 1 of 5/i)).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it("Cancel fires a session.recover ws_request with action cancel_retry", async () => {
    renderAction(retryMessage(), "WAITING_FOR_INPUT");
    fireEvent.click(screen.getByTestId(CANCEL_TEST_ID));
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    expect(requestMock).toHaveBeenCalledWith(SESSION_RECOVER_METHOD, {
      task_id: TEST_TASK_ID,
      session_id: TEST_SESSION_ID,
      action: "cancel_retry",
    });
  });

  it("hides while the session is RUNNING (retry in flight) to avoid a stale card", () => {
    const { container } = renderAction(retryMessage(), "RUNNING");
    expect(container.firstChild).toBeNull();
  });

  it("hides while the session is STARTING so the startup status remains visible", () => {
    const { container } = renderAction(retryMessage(), "STARTING");
    expect(container.firstChild).toBeNull();
  });

  it("renders the red variant for a non-warning recovery banner", () => {
    const errorMsg = recoveryMessage();
    renderAction(errorMsg, "WAITING_FOR_INPUT", "agent process exited unexpectedly");
    const text = screen.getByText(/Agent encountered an error/i);
    expect(text.className).toContain("text-red-600");
    expect(text.className).not.toContain("text-amber-600");
  });

  it("keeps a recovery card visible while the session is waiting, even without an error_message", () => {
    const errorMsg = recoveryMessage();

    renderAction(errorMsg, "WAITING_FOR_INPUT", "");
    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });
});

describe("ActionMessage recovery ownership", () => {
  it("keeps the recovery entry after its Resume request succeeds and removes controls", async () => {
    const errorMsg = recoveryMessage(true);

    renderAction(errorMsg, "WAITING_FOR_INPUT", "");
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy());
    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
  });

  it("shows recovery controls only for the current stamped failure", () => {
    const historical = recoveryMessage(true);
    historical.metadata = {
      ...(historical.metadata as Record<string, unknown>),
      error_stamp: HISTORICAL_FAILURE_STAMP,
    };
    const current = recoveryMessage(true);
    current.metadata = {
      ...(current.metadata as Record<string, unknown>),
      error_stamp: CURRENT_FAILURE_STAMP,
    };
    const { rerender } = renderAction(historical, "WAITING_FOR_INPUT", "", undefined, {
      last_agent_error: {
        message: "The newer session failure.",
        stamp: CURRENT_FAILURE_STAMP,
      },
    });

    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();

    rerender(<ActionMessage comment={current} />);

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("does not restore controls for a stamped history row after a different failure was dismissed", () => {
    const historical = recoveryMessage(true);
    historical.metadata = {
      ...(historical.metadata as Record<string, unknown>),
      error_stamp: HISTORICAL_FAILURE_STAMP,
    };

    renderAction(historical, "WAITING_FOR_INPUT", "", undefined, {
      last_agent_error: {
        message: "A newer failure.",
        stamp: NEW_FAILURE_STAMP,
        dismissed_at: RECOVERY_RESOLVED_AT,
      },
    });

    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
  });

  it("keeps controls for an unstamped legacy row matching the current failure", () => {
    const legacy = recoveryMessage(true);
    legacy.content = "Agent encountered an error: The agent could not start.";
    legacy.metadata = {
      ...(legacy.metadata as Record<string, unknown>),
      error_stamp: undefined,
    };

    renderAction(legacy, "WAITING_FOR_INPUT", "", undefined, {
      last_agent_error: {
        message: "The agent could not start.",
        occurred_at: legacy.created_at,
      },
    });

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("keeps the historical recovery entry after a successful resume settles back to waiting", async () => {
    const errorMsg = recoveryMessage(true);

    const { setSessionState } = renderActionWithStore(errorMsg, "WAITING_FOR_INPUT", "");
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy());

    // A successful resume drives the session through an active state (which
    // hides the card via isSessionActive) and then back to WAITING_FOR_INPUT
    // once the agent is idle again. The recovery acknowledgment must survive
    // that intermediate unmount so the card does not reappear until the user
    // actually sends the next message.
    setSessionState("STARTING");
    setSessionState("WAITING_FOR_INPUT");

    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
  });

  it("keeps a recovery card visible when the WebSocket client is unavailable", () => {
    getWebSocketClientMock.mockReturnValue(null);
    renderAction(recoveryMessage(true), "WAITING_FOR_INPUT", "");

    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
  });
});

describe("ActionMessage recovery settlement", () => {
  it("marks only the matching history row resolved by authoritative stamp settlement", () => {
    const oldError = recoveryHistoryMessage(HISTORICAL_FAILURE_STAMP);
    const success = providerRestoredSuccessMessage(HISTORICAL_FAILURE_STAMP);
    renderRecoveryHistory(oldError, [oldError, success], {
      recovery_resolutions: [
        {
          error_stamp: HISTORICAL_FAILURE_STAMP,
          attempt_id: "resume-1",
          resolved_at: RECOVERY_RESOLVED_AT,
        },
      ],
    });

    expect(screen.getByTestId("session-recovery-resolved").textContent).toBe("Resolved");
    expect(screen.queryByTestId("session-recovery-dismissed")).toBeNull();
    expect(screen.getByTestId("session-recovery-history").querySelector("p")?.textContent).toBe(
      "The requested model was not available to the agent.",
    );
  });

  it("shows manual dismissal separately when no matching success notice exists", () => {
    const oldError = recoveryHistoryMessage(HISTORICAL_FAILURE_STAMP);
    const unrelatedSuccess = providerRestoredSuccessMessage("failure-other");
    renderRecoveryHistory(oldError, [oldError, unrelatedSuccess]);

    expect(screen.getByTestId("session-recovery-dismissed").textContent).toBe("Dismissed");
    expect(screen.queryByTestId("session-recovery-resolved")).toBeNull();
  });
});

describe("ActionMessage active legacy recovery ownership", () => {
  it("keeps an active legacy recovery row compact when the owner has no stamp", () => {
    const legacyMessage = recoveryMessage();
    legacyMessage.id = "legacy-recovery-owner";
    legacyMessage.metadata = {
      variant: "error",
      recovery_actions: true,
      error_output: "The provider request failed.",
      actions: [
        {
          type: "archive_task",
          label: "Archive task",
          test_id: "legacy-recovery-archive-button",
        },
      ],
    };
    const session = {
      id: TEST_SESSION_ID,
      task_id: TEST_TASK_ID,
      state: "FAILED",
      error_message: "",
      metadata: {},
    } as unknown as TaskSession;

    render(
      <StateProvider
        initialState={
          {
            taskSessions: { items: { [TEST_SESSION_ID]: session } },
            messages: { bySession: { [TEST_SESSION_ID]: [legacyMessage] }, metaBySession: {} },
          } as Partial<AppState>
        }
      >
        <SessionRecoveryProvider
          session={session}
          messages={[legacyMessage]}
          taskId={TEST_TASK_ID}
          enabled
        >
          <ActionMessage comment={legacyMessage} />
        </SessionRecoveryProvider>
      </StateProvider>,
    );

    expect(screen.getByTestId("session-recovery-history").textContent).toContain(
      "This failure is explained in the recovery card above.",
    );
    expect(screen.queryByTestId("legacy-recovery-archive-button")).toBeNull();
  });
});

describe("ActionMessage historical typed recovery evidence", () => {
  it("renders same-text historical failures from their own evidence after a successor replaces the current error", async () => {
    const first = recoveryHistoryMessage("failure-first");
    first.created_at = "2026-09-29T09:00:00Z";
    first.metadata = {
      ...(first.metadata as Record<string, unknown>),
      phase: "bootstrap",
      attempt_id: "resume-1",
      execution_id: "650e8400-e29b-41d4-a716-446655440000",
      causes: [
        {
          operation: "resume",
          code: "model_unavailable",
          reason: "requested_not_advertised",
          requested_model: "first-model",
          detail: "The saved model was not advertised.",
        },
      ],
      error_output: "token=old-private-value",
    };
    const second = recoveryHistoryMessage("failure-second");
    second.id = "recovery-second";
    second.metadata = {
      ...(second.metadata as Record<string, unknown>),
      phase: "bootstrap",
      attempt_id: "resume-2",
      causes: [
        {
          operation: "resume",
          code: "model_selection_failed",
          reason: "application_failed",
          requested_model: "primary-model",
          attempted_model: "alternate-model",
          detail: "The attempted model could not be applied.",
        },
      ],
    };
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    renderRecoveryHistory([first, second], [first, second], {
      last_agent_error: {
        message: "A successor failure.",
        stamp: "successor-failure",
        occurred_at: "2026-09-29T10:06:00Z",
      },
      recovery_resolutions: [
        {
          error_stamp: "failure-first",
          attempt_id: "resume-1",
          resolved_at: RECOVERY_RESOLVED_AT,
        },
      ],
    });

    const rows = screen.getAllByTestId("session-recovery-history");
    expect(rows).toHaveLength(2);
    expect(screen.getByTestId("session-recovery-resolved").textContent).toBe("Resolved");
    expect(rows[0].textContent).toContain("first-model");
    expect(rows[1].textContent).toContain("alternate-model");
    expect(rows[1].textContent).not.toContain("first-model");

    const firstDetails = rows[0].querySelector("summary");
    fireEvent.click(firstDetails!);
    const displayed = rows[0].querySelector("pre")?.textContent ?? "";
    expect(displayed).toContain("Attempt: resume-1");
    expect(displayed).toContain("Requested model: first-model");
    expect(displayed).toContain("Phase: bootstrap");
    expect(displayed).not.toContain("old-private-value");
    fireEvent.click(screen.getAllByRole("button", { name: "Copy details" })[0]);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(displayed));
  });
});

function recoveryHistoryMessage(stamp: string): Message {
  const message = recoveryMessage();
  return {
    ...message,
    content: RECOVERY_MESSAGE,
    created_at: "2026-09-29T10:00:00Z",
    metadata: {
      ...(message.metadata as Record<string, unknown>),
      recovery_actions: true,
      error_stamp: stamp,
      causes: [{ operation: "resume", code: "model_unavailable" }],
    },
  };
}

function providerRestoredSuccessMessage(resolvedErrorStamp: string): Message {
  return {
    ...recoveryMessage(),
    id: `success-${resolvedErrorStamp}`,
    type: "status",
    created_at: RECOVERY_RESOLVED_AT,
    content: "Session resumed.",
    metadata: {
      variant: "resume_settings_provider_restored",
      resolved_error_stamp: resolvedErrorStamp,
    },
  };
}

function renderRecoveryHistory(
  comment: Message | Message[],
  messages: Message[],
  metadataOverrides: Record<string, unknown> = {},
) {
  const comments = Array.isArray(comment) ? comment : [comment];
  const activeComment = comments[0];
  const session = {
    id: TEST_SESSION_ID,
    task_id: TEST_TASK_ID,
    state: "WAITING_FOR_INPUT",
    error_message: "",
    metadata: {
      last_agent_error: {
        message: RECOVERY_MESSAGE,
        stamp: (activeComment.metadata as Record<string, unknown>).error_stamp,
        dismissed_at: "2026-09-29T10:01:00Z",
        occurred_at: activeComment.created_at,
        causes: [{ operation: "resume", code: "model_unavailable" }],
      },
      recovery_resolved_at: RECOVERY_RESOLVED_AT,
      ...metadataOverrides,
    },
  } as unknown as TaskSession;
  return render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { [TEST_SESSION_ID]: session } },
          messages: { bySession: { [TEST_SESSION_ID]: messages }, metaBySession: {} },
        } as Partial<AppState>
      }
    >
      <SessionRecoveryProvider session={session} messages={messages} taskId={TEST_TASK_ID} enabled>
        {comments.map((item) => (
          <ActionMessage key={item.id} comment={item} />
        ))}
      </SessionRecoveryProvider>
    </StateProvider>,
  );
}

describe("ActionMessage — agent transport lost", () => {
  it("renders the agent-transport-lost reason for a dropped ACP connection", () => {
    renderAction(
      retryMessage({
        content: "Agent connection lost",
        metadata: {
          ...transientRetryMetadata(1, 5),
          failure_code: "agent_transport_lost",
        },
      }),
      "WAITING_FOR_INPUT",
    );
    expect(screen.getByText(/Agent connection lost/i)).toBeTruthy();
  });
});

describe("ActionMessage — retry schedule updates", () => {
  it("resets the fallback countdown when a later retry schedule arrives", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-08T12:00:00.000Z"));
    try {
      const { rerender } = renderAction(
        retryMessage({ metadata: transientRetryMetadata(1, 5) }),
        "WAITING_FOR_INPUT",
      );
      expect(screen.getByText(/retrying in 0:05/i)).toBeTruthy();
      rerender(
        <ActionMessage comment={retryMessage({ metadata: transientRetryMetadata(2, 10) })} />,
      );
      expect(screen.getByText(/retrying in 0:10/i)).toBeTruthy();
      expect(screen.getByText(/attempt 2 of 5/i)).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("ActionMessage — running stall notice", () => {
  it("renders a neutral compact notice only while the session is running", () => {
    renderAction(stalledMessage(), "RUNNING", undefined, "turn-1");

    const notice = screen.getByTestId("running-action-notice");
    expect(notice.className).toContain("text-muted-foreground");
    expect(notice.className).not.toContain("text-amber");
    expect(notice.className).not.toContain("text-red");
    expect(notice.querySelector("svg")).toBeNull();
    const button = screen.getByTestId(STALL_CANCEL_TEST_ID);
    expect(button.className).toContain("h-6");
    expect(button.className).toContain("max-md:min-h-11");
    expect(button.className).not.toContain("w-full");
  });

  it("hides the running-only notice after the session settles", () => {
    const { container } = renderAction(stalledMessage(), "WAITING_FOR_INPUT");
    expect(container.firstChild).toBeNull();
  });

  it("keeps a terminal stall diagnostic visible after the session fails", () => {
    const message = stalledMessage();
    message.type = "error";

    renderAction(message, "FAILED");

    expect(screen.getByText("Still waiting on Start dev server.")).toBeTruthy();
  });

  it("sends agent.cancel when Cancel turn is activated", async () => {
    renderAction(stalledMessage(), "RUNNING", undefined, "turn-1");

    fireEvent.click(screen.getByTestId(STALL_CANCEL_TEST_ID));

    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith("agent.cancel", { session_id: TEST_SESSION_ID }),
    );
  });

  it("hides an old notice when a later turn is active", () => {
    const { container } = renderAction(stalledMessage("turn-1"), "RUNNING", undefined, "turn-2");
    expect(container.firstChild).toBeNull();
  });

  // @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.6
  it.each(["loaded", "unloaded"])(
    "hides a running notice when a %s compaction tool resumes in the same turn",
    (toolWindow) => {
      const notice = stalledMessage();
      const { addMessage, updateMessage } = renderActionWithStore(notice, "RUNNING", "", "turn-1");
      const tool: Message = {
        ...notice,
        id: "compaction-tool",
        author_type: "agent",
        type: "tool_call",
        content: "Compact conversation",
        created_at: "2026-05-29T23:59:00Z",
        updated_at: "2026-05-29T23:59:00Z",
        metadata: { tool_call_id: "compact-1", status: "running" },
      };
      if (toolWindow === "loaded") addMessage(tool);
      else addMessage(notice);
      expect(screen.queryByTestId("running-action-notice")).not.toBeNull();

      updateMessage({
        ...tool,
        updated_at: "2026-05-30T00:00:01Z",
        metadata: { ...tool.metadata, status: "complete" },
      });

      expect(screen.queryByTestId("running-action-notice")).toBeNull();
    },
  );

  it("hides a running-only notice without a turn ID", () => {
    const message = stalledMessage();
    message.turn_id = undefined;

    const { container } = renderAction(message, "RUNNING", undefined, "turn-1");

    expect(container.firstChild).toBeNull();
  });
});

describe("ActionMessage — missing PR branch", () => {
  it("renders a plain-language recovery panel with collapsed technical details", () => {
    renderAction(
      retryMessage({
        content:
          'The remote PR branch "codex/enhance-prompt-result-delivery" no longer exists (likely merged and deleted).',
        metadata: {
          variant: "warning",
          failure_kind: "missing_pr_branch",
          missing_branch: "codex/enhance-prompt-result-delivery",
          error_output: "fatal: unable to access github.com: Could not resolve host",
          actions: [
            {
              type: "archive_task",
              label: "Archive task",
              icon: "archive",
              test_id: "missing-branch-archive-button",
            },
            {
              type: "delete_task",
              label: "Delete task",
              icon: "trash",
              variant: "destructive",
              test_id: "missing-branch-delete-button",
            },
          ],
        },
      } as Partial<Message>),
      "FAILED",
    );

    expect(screen.getByTestId("missing-branch-recovery")).toBeTruthy();
    expect(screen.getByText("Branch is no longer available")).toBeTruthy();
    expect(screen.getByText("codex/enhance-prompt-result-delivery")).toBeTruthy();
    const technicalDetails = screen.getByText(TECHNICAL_DETAILS).closest("details");
    expect(technicalDetails?.open).toBe(false);
    expect(screen.getByTestId("missing-branch-archive-button").className).toContain("h-7");
    expect(screen.getByTestId("missing-branch-delete-button").className).toContain("h-7");

    fireEvent.click(screen.getByText(TECHNICAL_DETAILS));
    expect(technicalDetails?.open).toBe(true);
    expect(screen.getByText(/Could not resolve host/)).toBeTruthy();
  });

  it("uses the current session error as collapsed technical details", () => {
    renderAction(
      retryMessage({
        content: 'The remote PR branch "feature/missing" no longer exists.',
        metadata: {
          variant: "warning",
          failure_kind: "missing_pr_branch",
          missing_branch: "feature/missing",
          actions: [
            {
              type: "archive_task",
              label: "Archive task",
              test_id: "missing-branch-archive-button",
            },
          ],
        },
      } as Partial<Message>),
      "FAILED",
      "environment preparation failed: fatal: could not resolve host github.com",
    );

    const details = screen.getByText(TECHNICAL_DETAILS).closest("details");
    expect(details?.open).toBe(false);
    expect(screen.getByText(/could not resolve host github.com/)).toBeTruthy();

    fireEvent.click(screen.getByText(TECHNICAL_DETAILS));
    expect(details?.open).toBe(true);
  });
});

describe("ActionMessage — provider quota recovery", () => {
  it("renders localized model/reset guidance with collapsed sanitized details", () => {
    renderAction(
      retryMessage({
        content: "provider quota reached",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "provider_quota_limited",
          provider_name: "OpenCode",
          model_id: "kimi-k3",
          reset_at: "2026-08-02T19:34:44Z",
          error_output: "5-hour usage limit reached",
          actions: [
            {
              type: "ws_request",
              label: RESUME_LABEL,
              test_id: RESUME_TEST_ID,
            },
          ],
        },
      } as Partial<Message>),
      "WAITING_FOR_INPUT",
    );

    expect(screen.getByTestId("provider-quota-recovery")).toBeTruthy();
    expect(screen.getByText(/OpenCode usage limit reached/i)).toBeTruthy();
    expect(screen.getByText(/kimi-k3/i)).toBeTruthy();
    const details = screen.getByText(TECHNICAL_DETAILS).closest("details");
    expect(details?.open).toBe(false);
    expect(screen.getByTestId(RESUME_TEST_ID).className).toContain("h-7");
    expect(screen.getByTestId(RESUME_TEST_ID).className).toContain("max-md:h-11");

    fireEvent.click(screen.getByText(TECHNICAL_DETAILS));
    expect(details?.open).toBe(true);
    expect(screen.getByText("5-hour usage limit reached")).toBeTruthy();
    expect(screen.queryByText(/opencode\.ai\/workspace/i)).toBeNull();
  });

  it("uses a safe generic reset message when reset time is absent", () => {
    renderAction(
      retryMessage({
        content: "provider quota reached",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "provider_quota_limited",
          provider_name: "OpenCode",
          model_id: "kimi-k3",
          actions: [],
        },
      } as Partial<Message>),
      "WAITING_FOR_INPUT",
    );

    expect(screen.getByTestId("provider-quota-recovery")).toBeTruthy();
    expect(screen.getByText(/when the provider makes capacity available/i)).toBeTruthy();
  });
});

describe("ActionMessage — managed runtime startup recovery", () => {
  it("shows typed early-exit cause and actual attempts in the startup recovery card", () => {
    renderAction(
      retryMessage({
        type: "error",
        content: "managed runtime startup failed",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "managed_runtime_startup",
          startup_reason: "early_exit",
          startup_attempts: 2,
          error_output: "reason=early_exit attempts=2",
          actions: [
            {
              type: "ws_request",
              label: "Retry runtime",
              test_id: MANAGED_RUNTIME_RETRY_TEST_ID,
              params: {
                method: SESSION_RECOVER_METHOD,
                payload: {
                  task_id: TEST_TASK_ID,
                  session_id: TEST_SESSION_ID,
                  action: "runtime_retry",
                },
              },
            },
          ],
        },
      } as Partial<Message>),
      "FAILED",
    );

    const card = screen.getByTestId("managed-runtime-startup-recovery");
    expect(card.textContent).toContain("Agent stopped during startup");
    expect(card.textContent).toContain(
      "The agent process exited before initialization. Startup was attempted 2 times.",
    );
    expect(screen.getAllByTestId(MANAGED_RUNTIME_RETRY_TEST_ID)).toHaveLength(1);
    expect(screen.getByText(TECHNICAL_DETAILS).closest("details")?.open).toBe(false);
  });
});

describe("ActionMessage — managed npm runtime recovery", () => {
  it("renders one localized retry action with collapsed technical details", async () => {
    renderAction(
      retryMessage({
        content: "managed runtime failed",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "managed_runtime_npm_resolution",
          error_output: "npm error code ETARGET\nnpm error notarget No matching version found",
          actions: [
            {
              type: "ws_request",
              label: "backend label is ignored",
              test_id: MANAGED_RUNTIME_RETRY_TEST_ID,
              params: {
                method: SESSION_RECOVER_METHOD,
                payload: {
                  task_id: TEST_TASK_ID,
                  session_id: TEST_SESSION_ID,
                  action: "runtime_retry",
                },
              },
            },
          ],
        },
      } as Partial<Message>),
      "WAITING_FOR_INPUT",
    );

    const card = screen.getByTestId("managed-runtime-npm-recovery");
    expect(card.textContent).toContain("npm could not prepare the runtime");
    expect(card.textContent).toContain("Kandev refreshed package data");
    expect(card.textContent).not.toMatch(/ACP/i);
    expect(screen.getByText(TECHNICAL_DETAILS).closest("details")?.open).toBe(false);
    expect(
      screen
        .getAllByRole("button")
        .filter((button) => button.getAttribute("data-testid") === MANAGED_RUNTIME_RETRY_TEST_ID),
    ).toHaveLength(1);
    expect(screen.getByTestId(MANAGED_RUNTIME_RETRY_TEST_ID).textContent).toContain(
      "Retry runtime",
    );

    fireEvent.click(screen.getByTestId(MANAGED_RUNTIME_RETRY_TEST_ID));
    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(SESSION_RECOVER_METHOD, {
        task_id: TEST_TASK_ID,
        session_id: TEST_SESSION_ID,
        action: "runtime_retry",
      }),
    );
  });

  it("explains when npm's release-age policy blocks the selected runtime", () => {
    renderAction(
      retryMessage({
        content: "managed runtime is blocked by npm policy",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "managed_runtime_npm_policy",
          error_output:
            "npm error notarget No matching version found for @example/agent@1.2.3. A minimum release age policy is in effect.\n  @example/agent@1.2.3 release date: <release-date>",
          actions: [
            {
              type: "ws_request",
              label: "backend label is ignored",
              test_id: MANAGED_RUNTIME_RETRY_TEST_ID,
              params: {
                method: SESSION_RECOVER_METHOD,
                payload: {
                  task_id: TEST_TASK_ID,
                  session_id: TEST_SESSION_ID,
                  action: "runtime_retry",
                },
              },
            },
          ],
        },
      } as Partial<Message>),
      "WAITING_FOR_INPUT",
    );

    const card = screen.getByTestId("managed-runtime-npm-recovery");
    expect(card.textContent).toContain("npm blocked this runtime version");
    expect(card.textContent).toContain(
      "Check npm's min-release-age or before setting. Wait until this version is eligible or select an older version, then retry.",
    );
    expect(card.textContent).not.toContain("refreshed package data");
    expect(card.textContent).not.toContain("2026-09-20T10:30:00Z");
    expect(screen.getByText(TECHNICAL_DETAILS).closest("details")?.open).toBe(false);
    expect(screen.getAllByTestId(MANAGED_RUNTIME_RETRY_TEST_ID)).toHaveLength(1);
    expect(screen.getByTestId(MANAGED_RUNTIME_RETRY_TEST_ID).textContent).toContain(
      "Retry runtime",
    );
  });
});

describe("ActionMessage — remediation link", () => {
  const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";
  const QUOTA_OUTPUT = "5-hour usage limit reached";

  /** Builds a recovery Message carrying the given remediation URL in its metadata. */
  function recoveryMeta(remediationUrl?: string): Message {
    return retryMessage({
      content: RECOVERY_MESSAGE,
      metadata: {
        variant: "error",
        recovery_actions: true,
        remediation_url: remediationUrl,
        error_output: "usage limit reached",
        actions: [{ type: "ws_request", label: RESUME_LABEL, test_id: RESUME_TEST_ID }],
      },
    } as Partial<Message>);
  }

  it("renders a validated remediation link for quota recovery", () => {
    renderAction(
      retryMessage({
        content: "provider quota reached",
        metadata: {
          variant: "error",
          recovery_actions: true,
          failure_kind: "provider_quota_limited",
          provider_name: "OpenCode",
          error_output: QUOTA_OUTPUT,
          remediation_url: REMEDIATION_URL,
          actions: [{ type: "ws_request", label: RESUME_LABEL, test_id: RESUME_TEST_ID }],
        },
      } as Partial<Message>),
      "WAITING_FOR_INPUT",
    );

    const link = screen.getByTestId("remediation-link") as HTMLAnchorElement;
    expect(link.href).toBe(REMEDIATION_URL);
    expect(link.target).toBe("_blank");
    expect(link.rel).toBe("noopener noreferrer");
    expect(link.className).toContain("h-7");
    expect(link.className).toContain("max-md:h-11");
    // The sanitized message and collapsed details stay URL-free.
    expect(screen.queryByText(/opencode\.ai\/workspace/i)).toBeNull();
    expect(screen.getByTestId("provider-quota-recovery").textContent).toContain(QUOTA_OUTPUT);
  });

  it("renders a remediation link for a generic recovery card too", () => {
    renderAction(recoveryMeta(REMEDIATION_URL), "WAITING_FOR_INPUT");

    const link = screen.getByTestId("remediation-link") as HTMLAnchorElement;
    expect(link.href).toBe(REMEDIATION_URL);
    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("renders no link for an invalid remediation URL", () => {
    renderAction(
      recoveryMeta("https://evil.example.com/workspace/wrk_123/go"),
      "WAITING_FOR_INPUT",
    );

    expect(screen.queryByTestId("remediation-link")).toBeNull();
    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });
});

it("moves unsafe long legacy summaries into redacted technical details", () => {
  const comment = {
    ...recoveryMessage(true),
    content: "failed: token=synthetic-private-value\n" + "nested diagnostic ".repeat(100),
  };
  const { container } = renderAction(comment, "FAILED");
  expect(container.textContent).not.toContain("synthetic-private-value");
  expect(screen.getByText("An error occurred")).toBeTruthy();
  expect(container.querySelector("pre")?.textContent).toContain("nested diagnostic");
});
