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

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const CANCEL_TEST_ID = "recovery-cancel-retry-button";
const TECHNICAL_DETAILS = "Technical details";

function retryMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    author_type: "system",
    content: "Provider overloaded — retrying in 5s (attempt 1/3)",
    type: "status",
    created_at: "2026-05-30T00:00:00Z",
    metadata: {
      variant: "warning",
      retrying: true,
      attempt: 1,
      max_attempts: 3,
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

/** ActionMessage reads session state from the store (keyed by comment.session_id),
 *  so seed it via the provider instead of passing a prop. */
function renderAction(comment: Message, sessionState?: TaskSessionState, sessionError?: string) {
  const initialState: Partial<AppState> = sessionState
    ? {
        taskSessions: {
          items: {
            "sess-1": { state: sessionState, error_message: sessionError } as TaskSession,
          },
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
    const text = screen.getByText(/retrying in 5s \(attempt 1\/3\)/i);
    expect(text.className).toContain("text-amber-600");
    expect(text.className).not.toContain("text-red-600");
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
    const errorMsg = retryMessage({
      content: "Agent encountered an error",
      metadata: {
        variant: "error",
        recovery_actions: true,
        actions: [
          { type: "ws_request", label: "Resume session", test_id: "recovery-resume-button" },
        ],
      },
    } as Partial<Message>);
    renderAction(errorMsg, "WAITING_FOR_INPUT");
    const text = screen.getByText(/Agent encountered an error/i);
    expect(text.className).toContain("text-red-600");
    expect(text.className).not.toContain("text-amber-600");
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
