import type { PropsWithChildren } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import {
  getStoredAcknowledgedAgentErrors,
  lastAgentErrorStamp,
  setStoredAcknowledgedAgentErrors,
} from "@/lib/session-last-agent-error";
import { sessionId, taskId, type Message, type TaskSession } from "@/lib/types/http";
import { usePersistResolvedAgentErrorAcknowledgements } from "./use-agent-error-acknowledgements";

const ERROR_OCCURRED_AT = "2026-06-14T10:00:00Z";
const AGENT_MESSAGE_AFTER_ERROR_AT = "2026-06-14T10:00:01Z";
const ERROR_MESSAGE = "agent failed";
const ERROR_STAMP = lastAgentErrorStamp({
  message: ERROR_MESSAGE,
  occurredAt: ERROR_OCCURRED_AT,
});

function wrapper() {
  return function Wrapper({ children }: PropsWithChildren) {
    return <StateProvider>{children}</StateProvider>;
  };
}

function session(id: string, metadata: TaskSession["metadata"]): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: ERROR_OCCURRED_AT,
    updated_at: ERROR_OCCURRED_AT,
    metadata,
  } as TaskSession;
}

function sessionWithError(id: string): TaskSession {
  return session(id, {
    last_agent_error: {
      message: ERROR_MESSAGE,
      occurred_at: ERROR_OCCURRED_AT,
    },
  });
}

function agentMessage(session: string, createdAt: string): Message {
  return {
    id: `message-${session}`,
    session_id: sessionId(session),
    task_id: taskId("task-1"),
    author_type: "agent",
    content: "",
    type: "agent_message",
    created_at: createdAt,
  } as unknown as Message;
}

describe("usePersistResolvedAgentErrorAcknowledgements", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
  });

  it("acknowledges sessions whose error is followed by an agent message", async () => {
    renderHook(
      () =>
        usePersistResolvedAgentErrorAcknowledgements({
          sessionsById: { "session-1": sessionWithError("session-1") },
          sessionIds: ["session-1"],
          messagesBySession: {
            "session-1": [agentMessage("session-1", AGENT_MESSAGE_AFTER_ERROR_AT)],
          },
          dismissedAgentErrors: {},
        }),
      { wrapper: wrapper() },
    );

    await waitFor(() => {
      expect(getStoredAcknowledgedAgentErrors()).toEqual({ "session-1": ERROR_STAMP });
    });
  });

  it("does not rewrite a stamp that is already acknowledged", async () => {
    setStoredAcknowledgedAgentErrors({ "session-1": ERROR_STAMP });
    const setItem = vi.spyOn(Storage.prototype, "setItem");

    renderHook(
      () =>
        usePersistResolvedAgentErrorAcknowledgements({
          sessionsById: { "session-1": sessionWithError("session-1") },
          sessionIds: ["session-1"],
          messagesBySession: {
            "session-1": [agentMessage("session-1", AGENT_MESSAGE_AFTER_ERROR_AT)],
          },
          dismissedAgentErrors: {},
        }),
      { wrapper: wrapper() },
    );

    await new Promise((resolve) => window.setTimeout(resolve, 0));
    expect(setItem).not.toHaveBeenCalled();
  });

  it("does not acknowledge when the error is not followed by an agent message", async () => {
    renderHook(
      () =>
        usePersistResolvedAgentErrorAcknowledgements({
          sessionsById: { "session-1": sessionWithError("session-1") },
          sessionIds: ["session-1"],
          messagesBySession: {
            "session-1": [agentMessage("session-1", ERROR_OCCURRED_AT)],
          },
          dismissedAgentErrors: {},
        }),
      { wrapper: wrapper() },
    );

    await waitFor(() => {
      expect(getStoredAcknowledgedAgentErrors()).toEqual({});
    });
  });

  it("does not acknowledge a session with no last agent error", async () => {
    renderHook(
      () =>
        usePersistResolvedAgentErrorAcknowledgements({
          sessionsById: { "session-1": session("session-1", {}) },
          sessionIds: ["session-1"],
          messagesBySession: {
            "session-1": [agentMessage("session-1", AGENT_MESSAGE_AFTER_ERROR_AT)],
          },
          dismissedAgentErrors: {},
        }),
      { wrapper: wrapper() },
    );

    await waitFor(() => {
      expect(getStoredAcknowledgedAgentErrors()).toEqual({});
    });
  });
});
