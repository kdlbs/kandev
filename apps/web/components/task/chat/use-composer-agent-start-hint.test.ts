import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

let mockResumeSkippedSessionIds: Record<string, true> = {};

type MockState = {
  tasks: { resumeSkippedSessionIds: Record<string, true> };
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) =>
    selector({ tasks: { resumeSkippedSessionIds: mockResumeSkippedSessionIds } }),
}));

import { useComposerAgentStartHint } from "./use-composer-agent-start-hint";

const SESSION_ID = "session-1";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId("task-1"),
    author_type: "user",
    type: "text",
    created_at: "2026-07-22T00:00:00Z",
    content: "hello",
    ...overrides,
  } as Message;
}

afterEach(cleanup);

beforeEach(() => {
  mockResumeSkippedSessionIds = {};
});

describe("useComposerAgentStartHint", () => {
  it("shows for a resume-skipped WAITING_FOR_INPUT session without recovery actions", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    const { result } = renderHook(() =>
      useComposerAgentStartHint(SESSION_ID, "WAITING_FOR_INPUT", [], []),
    );
    expect(result.current).toBe(true);
  });

  it("hides when the session is not marked resume-skipped", () => {
    const { result } = renderHook(() =>
      useComposerAgentStartHint(SESSION_ID, "WAITING_FOR_INPUT", [], []),
    );
    expect(result.current).toBe(false);
  });

  it("hides when no session is bound", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    const { result } = renderHook(() =>
      useComposerAgentStartHint(null, "WAITING_FOR_INPUT", [], []),
    );
    expect(result.current).toBe(false);
  });

  it("hides when a transcript message carries recovery actions", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    const recovery = message({
      metadata: { recovery_actions: true, actions: [{ type: "resume_session", label: "Resume" }] },
    });
    const { result } = renderHook(() =>
      useComposerAgentStartHint(SESSION_ID, "WAITING_FOR_INPUT", [recovery], []),
    );
    expect(result.current).toBe(false);
  });

  it("hides when a footer action message carries recovery actions", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    const recovery = message({
      metadata: { recovery_actions: true, actions: [{ type: "resume_session", label: "Resume" }] },
    });
    const { result } = renderHook(() =>
      useComposerAgentStartHint(SESSION_ID, "WAITING_FOR_INPUT", [], [recovery]),
    );
    expect(result.current).toBe(false);
  });

  it("hides for FAILED, STARTING, RUNNING and terminal sessions", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    for (const state of ["FAILED", "STARTING", "RUNNING", "CANCELLED", "COMPLETED"] as const) {
      const { result } = renderHook(() => useComposerAgentStartHint(SESSION_ID, state, [], []));
      expect(result.current).toBe(false);
    }
  });
});
