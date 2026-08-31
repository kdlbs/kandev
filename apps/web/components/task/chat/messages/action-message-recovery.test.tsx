import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { StoreApi } from "zustand";
import { ActionMessage } from "./action-message";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { WebSocketRequestError } from "@/lib/ws/client";

const requestMock = vi.fn().mockResolvedValue({});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const RECOVERY_MESSAGE = "Agent encountered an error";
const RESUME_TEST_ID = "recovery-resume-button";
const FRESH_TEST_ID = "recovery-fresh-button";
const BRANCH_FAILURE_MESSAGE = "The saved branch is no longer available.";
const RECOVERY_ERROR_TEST_ID = "session-recovery-error";
const TEST_SESSION_ID = "sess-1";
const TEST_TASK_ID = "task-1";
const FAILED_AT = "2026-05-30T00:00:00Z";
const BOOTED_AFTER_FAILURE = "2026-05-30T00:01:00Z";
const BOOTED_BEFORE_FAILURE = "2026-05-29T23:59:00Z";

function recoveryMessage(): Message {
  return {
    id: "msg-1",
    session_id: toSessionId(TEST_SESSION_ID),
    task_id: toTaskId(TEST_TASK_ID),
    author_type: "agent",
    content: RECOVERY_MESSAGE,
    type: "status",
    created_at: FAILED_AT,
    metadata: {
      variant: "error",
      recovery_actions: true,
      actions: [
        {
          type: "ws_request",
          label: "Resume session",
          test_id: RESUME_TEST_ID,
          params: {
            method: "session.recover",
            payload: { task_id: TEST_TASK_ID, session_id: TEST_SESSION_ID, action: "resume" },
          },
        },
        {
          type: "ws_request",
          label: "Start fresh session",
          test_id: FRESH_TEST_ID,
          params: {
            method: "session.recover",
            payload: { task_id: TEST_TASK_ID, session_id: TEST_SESSION_ID, action: "fresh_start" },
          },
        },
      ],
    },
  } as Message;
}

function bootMessage(createdAt: string, metadata: Record<string, unknown> = {}): Message {
  return {
    id: `boot-${createdAt}`,
    session_id: toSessionId(TEST_SESSION_ID),
    task_id: toTaskId(TEST_TASK_ID),
    author_type: "agent",
    content: "",
    type: "script_execution",
    created_at: createdAt,
    metadata: {
      script_type: "agent_boot",
      agent_name: "OpenCode",
      is_resuming: true,
      status: "exited",
      ...metadata,
    },
  } as Message;
}

/**
 * Renders the card against a seeded transcript and WITHOUT activating Resume.
 * That is the reported situation: the click acknowledgment is in-memory, so a
 * reload, a task switch, or an auto-resume-on-open all present the persisted
 * card with no acknowledgment at all.
 */
function renderWithTranscript(
  sessionState: TaskSessionState,
  messages: Message[],
  sessionMetadata?: Record<string, unknown>,
) {
  const initialState: Partial<AppState> = {
    taskSessions: {
      items: {
        [TEST_SESSION_ID]: { state: sessionState, metadata: sessionMetadata } as TaskSession,
      },
    },
    turns: {
      bySession: {},
      activeBySession: {},
      loadedBySession: {},
      reconcileEpochBySession: {},
      settledBoundaryBySession: {},
    },
    messages: { bySession: { [TEST_SESSION_ID]: messages }, metaBySession: {} },
  };
  return render(<ActionMessage comment={recoveryMessage()} />, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
}

describe("ActionMessage — recovery card retires once the agent is back", () => {
  it("hides the card when a resume re-established the agent after the failure", () => {
    // A resumed agent settles at WAITING_FOR_INPUT, not RUNNING, so the session
    // state alone never retires the card.
    renderWithTranscript("WAITING_FOR_INPUT", [bootMessage(BOOTED_AFTER_FAILURE)]);

    expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull();
    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(FRESH_TEST_ID)).toBeNull();
  });

  it("hides the card when a fresh start re-established the agent after the failure", () => {
    renderWithTranscript("WAITING_FOR_INPUT", [
      bootMessage(BOOTED_AFTER_FAILURE, { is_resuming: false }),
    ]);

    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
  });

  it("keeps the card visible when the resume attempt failed", () => {
    renderWithTranscript("WAITING_FOR_INPUT", [
      bootMessage(BOOTED_AFTER_FAILURE, { status: "failed" }),
    ]);

    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("keeps the card visible while the resume is still running", () => {
    renderWithTranscript("WAITING_FOR_INPUT", [
      bootMessage(BOOTED_AFTER_FAILURE, { status: "running" }),
    ]);

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("keeps the card visible when the only successful boot predates the failure", () => {
    renderWithTranscript("WAITING_FOR_INPUT", [bootMessage(BOOTED_BEFORE_FAILURE)]);

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("keeps the card visible when the transcript holds no boot record at all", () => {
    renderWithTranscript("WAITING_FOR_INPUT", []);

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
  });

  it("uses the durable session resolution when the boot record is missing", () => {
    renderWithTranscript("WAITING_FOR_INPUT", [], { recovery_resolved_at: BOOTED_AFTER_FAILURE });

    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(FRESH_TEST_ID)).toBeNull();
  });

  it("keeps a typed branch failure visible with explicit recovery choices", async () => {
    requestMock.mockRejectedValueOnce(
      new WebSocketRequestError(BRANCH_FAILURE_MESSAGE, "CONFLICT", {
        kind: "branch_unrecoverable",
        recovery_action: "resume_new_branch",
        original_branch: "feature/lost",
        base_branch: "main",
      }),
    );

    renderWithTranscript("WAITING_FOR_INPUT", []);
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));

    expect(await screen.findByTestId(RECOVERY_ERROR_TEST_ID)).toBeTruthy();
    expect(screen.getByText(BRANCH_FAILURE_MESSAGE)).toBeTruthy();
    expect(screen.getByTestId("recovery-new-branch-button")).toBeTruthy();
    expect(screen.getByTestId("recovery-restore-workspace-button")).toBeTruthy();
  });

  it("retains both causes when manual resume and read-only restore fail", async () => {
    requestMock
      .mockRejectedValueOnce(
        new WebSocketRequestError(BRANCH_FAILURE_MESSAGE, "CONFLICT", {
          kind: "branch_unrecoverable",
          recovery_action: "resume_new_branch",
          original_branch: "feature/lost",
          base_branch: "main",
        }),
      )
      .mockRejectedValueOnce(new Error("Workspace restore failed."));

    renderWithTranscript("WAITING_FOR_INPUT", []);
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    expect(await screen.findByTestId(RECOVERY_ERROR_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("recovery-restore-workspace-button"));

    const recoveryError = await screen.findByTestId(RECOVERY_ERROR_TEST_ID);
    expect(recoveryError.textContent).toContain(`Resume failed: ${BRANCH_FAILURE_MESSAGE}`);
    expect(recoveryError.textContent).toContain(
      "Workspace restore failed: Workspace restore failed.",
    );
    expect(screen.getByTestId("recovery-new-branch-button")).toBeTruthy();
  });

  it("does not offer branch continuation for an ordinary recovery failure", async () => {
    requestMock.mockRejectedValueOnce(new Error("Provider is unavailable"));

    renderWithTranscript("WAITING_FOR_INPUT", []);
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));

    expect(await screen.findByText("Provider is unavailable")).toBeTruthy();
    expect(screen.queryByTestId("recovery-new-branch-button")).toBeNull();
    expect(screen.getByTestId("recovery-restore-workspace-button")).toBeTruthy();
  });
});

describe("ActionMessage recovery retry", () => {
  it("disables Retry while a repeated recovery request is pending", async () => {
    let resolveRetry: (() => void) | undefined;
    requestMock.mockRejectedValueOnce(new Error("Initial resume failed")).mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveRetry = resolve;
        }),
    );

    renderWithTranscript("WAITING_FOR_INPUT", []);
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    expect(await screen.findByTestId(RECOVERY_ERROR_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("ensure-session-error-retry"));
    await waitFor(() =>
      expect((screen.getByTestId("ensure-session-error-retry") as HTMLButtonElement).disabled).toBe(
        true,
      ),
    );
    fireEvent.click(screen.getByTestId("ensure-session-error-retry"));
    expect(requestMock).toHaveBeenCalledTimes(2);

    resolveRetry?.();
    await waitFor(() => expect(screen.queryByTestId(RECOVERY_ERROR_TEST_ID)).toBeNull());
  });
});

/** Renders the card with a live store handle so a spec can land transcript rows and
 *  session transitions after the user has already pressed Resume. */
function renderWithLiveStore(messages: Message[]) {
  let store: StoreApi<AppState> | null = null;
  function CaptureStore() {
    store = useAppStoreApi();
    return null;
  }
  const initialState: Partial<AppState> = {
    taskSessions: {
      items: { [TEST_SESSION_ID]: { state: "WAITING_FOR_INPUT" } as TaskSession },
    },
    turns: {
      bySession: {},
      activeBySession: {},
      loadedBySession: {},
      reconcileEpochBySession: {},
      settledBoundaryBySession: {},
    },
    messages: { bySession: { [TEST_SESSION_ID]: messages }, metaBySession: {} },
  };
  render(
    <StateProvider initialState={initialState}>
      <CaptureStore />
      <ActionMessage comment={recoveryMessage()} />
    </StateProvider>,
  );
  return {
    setSessionState: (state: TaskSessionState) =>
      act(() => {
        store?.getState().setTaskSession({
          id: toSessionId(TEST_SESSION_ID),
          task_id: toTaskId(TEST_TASK_ID),
          state,
          started_at: "",
          updated_at: "",
        } as TaskSession);
      }),
    setMessages: (next: Message[]) =>
      act(() => {
        store?.getState().setMessages(toSessionId(TEST_SESSION_ID), next);
      }),
  };
}

describe("ActionMessage — a recovery that failed keeps its controls", () => {
  it("brings the card back when the boot after an accepted resume reports failure", async () => {
    const live = renderWithLiveStore([]);

    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull());

    // The launch was accepted, so the card hid; the agent then failed to come up.
    live.setSessionState("STARTING");
    live.setMessages([bootMessage(BOOTED_AFTER_FAILURE, { status: "failed" })]);
    live.setSessionState("WAITING_FOR_INPUT");

    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
    expect(screen.getByTestId(FRESH_TEST_ID)).toBeTruthy();
  });

  it("brings the card back when an accepted resume drives the session to FAILED", async () => {
    const live = renderWithLiveStore([]);

    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull());

    live.setSessionState("FAILED");

    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
    expect(screen.getByTestId(FRESH_TEST_ID)).toBeTruthy();
  });

  it("keeps the card retired when the accepted resume actually boots", async () => {
    const live = renderWithLiveStore([]);

    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull());

    live.setSessionState("STARTING");
    live.setMessages([bootMessage(BOOTED_AFTER_FAILURE)]);
    live.setSessionState("WAITING_FOR_INPUT");

    expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull();
    expect(screen.queryByTestId(RESUME_TEST_ID)).toBeNull();
  });
});
