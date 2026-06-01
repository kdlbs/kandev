import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { ActionMessage } from "./action-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

const requestMock = vi.fn().mockResolvedValue({});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const CANCEL_TEST_ID = "recovery-cancel-retry-button";

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

function Wrapper({ children }: { children: ReactNode }) {
  // ActionMessage → useTaskRemoval now reads task sessions from the TanStack
  // Query cache (post-migration), so it calls useQueryClient and needs a
  // QueryClientProvider in addition to the Zustand StateProvider.
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <StateProvider initialState={{}}>{children}</StateProvider>
    </QueryClientProvider>
  );
}

describe("ActionMessage — transient retry (warning variant)", () => {
  it("renders the retrying copy in amber, not red", () => {
    render(<ActionMessage comment={retryMessage()} sessionState="WAITING_FOR_INPUT" />, {
      wrapper: Wrapper,
    });
    const text = screen.getByText(/retrying in 5s \(attempt 1\/3\)/i);
    expect(text.className).toContain("text-amber-600");
    expect(text.className).not.toContain("text-red-600");
  });

  it("Cancel fires a session.recover ws_request with action cancel_retry", async () => {
    render(<ActionMessage comment={retryMessage()} sessionState="WAITING_FOR_INPUT" />, {
      wrapper: Wrapper,
    });
    fireEvent.click(screen.getByTestId(CANCEL_TEST_ID));
    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    expect(requestMock).toHaveBeenCalledWith("session.recover", {
      task_id: "task-1",
      session_id: "sess-1",
      action: "cancel_retry",
    });
  });

  it("hides while the session is RUNNING (retry in flight) to avoid a stale card", () => {
    const { container } = render(
      <ActionMessage comment={retryMessage()} sessionState="RUNNING" />,
      { wrapper: Wrapper },
    );
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
    render(<ActionMessage comment={errorMsg} sessionState="WAITING_FOR_INPUT" />, {
      wrapper: Wrapper,
    });
    const text = screen.getByText(/Agent encountered an error/i);
    expect(text.className).toContain("text-red-600");
    expect(text.className).not.toContain("text-amber-600");
  });
});
