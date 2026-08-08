import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ActionMessage } from "./action-message";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

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

function retryMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
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
      session_id: "sess-1",
      task_id: "task-1",
      actions: [
        {
          type: "ws_request",
          label: "Cancel",
          icon: "x",
          test_id: CANCEL_TEST_ID,
          params: {
            method: "session.recover",
            payload: { task_id: "task-1", session_id: "sess-1", action: "cancel_retry" },
          },
        },
      ],
    },
    ...overrides,
  } as Message;
}

function transientRetryMetadata(attempt: number, retryInSeconds: number) {
  return {
    variant: "warning",
    retrying: true,
    attempt,
    max_attempts: 5,
    retry_in_seconds: retryInSeconds,
    session_id: "sess-1",
    task_id: "task-1",
    actions: [],
  };
}

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
                  method: "session.recover",
                  payload: { task_id: "task-1", session_id: "sess-1" },
                },
              }
            : {}),
        },
      ],
    },
  } as Partial<Message>);
}

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
          params: { method: "agent.cancel", payload: { session_id: "sess-1" } },
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
) {
  const initialState: Partial<AppState> = sessionState
    ? {
        taskSessions: {
          items: {
            "sess-1": { state: sessionState, error_message: sessionError } as TaskSession,
          },
        },
        turns: {
          bySession: {},
          activeBySession: activeTurnId ? { "sess-1": activeTurnId } : {},
        },
      }
    : {};
  return render(<ActionMessage comment={comment} />, {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
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
            session_id: "sess-1",
            task_id: "task-1",
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
    expect(requestMock).toHaveBeenCalledWith("session.recover", {
      task_id: "task-1",
      session_id: "sess-1",
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

  it("hides a recovery card after its Resume request succeeds", async () => {
    const errorMsg = recoveryMessage(true);

    renderAction(errorMsg, "WAITING_FOR_INPUT", "");
    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));
    await waitFor(() => expect(screen.queryByText(RECOVERY_MESSAGE)).toBeNull());
  });

  it("keeps a recovery card visible when the WebSocket client is unavailable", () => {
    getWebSocketClientMock.mockReturnValue(null);
    renderAction(recoveryMessage(true), "WAITING_FOR_INPUT", "");

    fireEvent.click(screen.getByTestId(RESUME_TEST_ID));

    expect(screen.getByTestId(RESUME_TEST_ID)).toBeTruthy();
    expect(screen.getByText(RECOVERY_MESSAGE)).toBeTruthy();
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
    expect(button.className).toContain("min-h-11");
    expect(button.className).not.toContain("w-full");
  });

  it("hides the running-only notice after the session settles", () => {
    const { container } = renderAction(stalledMessage(), "WAITING_FOR_INPUT");
    expect(container.firstChild).toBeNull();
  });

  it("sends agent.cancel when Cancel turn is activated", async () => {
    renderAction(stalledMessage(), "RUNNING", undefined, "turn-1");

    fireEvent.click(screen.getByTestId(STALL_CANCEL_TEST_ID));

    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith("agent.cancel", { session_id: "sess-1" }),
    );
  });

  it("hides an old notice when a later turn is active", () => {
    const { container } = renderAction(stalledMessage("turn-1"), "RUNNING", undefined, "turn-2");
    expect(container.firstChild).toBeNull();
  });

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
    expect(screen.getByTestId("missing-branch-archive-button").className).toContain("min-h-11");
    expect(screen.getByTestId("missing-branch-delete-button").className).toContain("min-h-11");

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
    expect(screen.getByTestId(RESUME_TEST_ID).className).toContain("min-h-11");

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

describe("ActionMessage — remediation link", () => {
  const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";
  const QUOTA_OUTPUT = "5-hour usage limit reached";

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
    expect(link.className).toContain("min-h-11");
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
