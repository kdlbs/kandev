import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { WebSocketRequestError } from "@/lib/ws/client";
import {
  beginSessionRecoveryGuidanceOperation,
  publishSessionRecoveryGuidance,
  selectSessionRecoveryGuidance,
  sessionRecoveryGuidanceIdentityFromSession,
} from "@/lib/session/session-recovery-guidance";
import { claimSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";
import { SessionRecoveryProvider, useSessionComposerRecovery } from "./session-recovery-context";

const mockLaunchErrorContext = vi.hoisted(() => ({ value: null as unknown }));

vi.mock("../task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => mockLaunchErrorContext.value,
}));

const session: TaskSession = {
  id: sessionId("session"),
  task_id: taskId("task"),
  state: "WAITING_FOR_INPUT",
  started_at: "2026-10-15T10:00:00Z",
  updated_at: "2026-10-15T10:00:00Z",
};

const recoveryBlock = {
  id: "block-1",
  incarnation_id: "incarnation-1",
  expected_generation: 2,
  reason: "unresolved_durable_work",
  consumer_reference: "agent_delivery",
  delivery_submission_id: "submission-1",
  updated_at: "2026-10-15T10:00:00Z",
};

function deliverySession(
  phase: "uncertain" | "restored",
  blocks: TaskSession["session_recovery_blocks"],
): TaskSession {
  return {
    ...session,
    state: phase === "restored" ? "RUNNING" : "WAITING_FOR_INPUT",
    metadata: {
      agent_delivery_recovery: {
        phase,
        revision: phase === "restored" ? 2 : 1,
        session_id: "session",
        agent_execution_id: "execution-1",
        submission_id: "submission-1",
        stream_id: "stream-1",
        incarnation_id: "incarnation-1",
        harness_generation: 2,
        prompt_generation: 1,
      },
    },
    session_recovery_blocks: blocks,
  };
}

beforeEach(() => {
  mockLaunchErrorContext.value = {
    automaticRecovery: {
      requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 1 },
      resumptionState: "error",
      error: "Session recovery failed",
    },
  };
});

function recoveryFor(current: TaskSession) {
  return renderHook(() => useSessionComposerRecovery("session"), {
    wrapper: recoveryWrapper(current),
  });
}

function recoveryWrapper(current: TaskSession) {
  return recoveryWrapperFor(() => current);
}

function recoveryWrapperFor(getSession: () => TaskSession) {
  return ({ children }: { children: ReactNode }) => {
    const current = getSession();
    return (
      <StateProvider initialState={{ taskSessions: { items: { [current.id]: current } } }}>
        <SessionRecoveryProvider session={current} messages={[]} taskId="task" enabled>
          {children}
        </SessionRecoveryProvider>
      </StateProvider>
    );
  };
}

describe("recovery model provenance", () => {
  it("identifies automatic request failure independently from a persisted session failure", () => {
    const { result } = recoveryFor(session);
    expect(result.current?.model).toMatchObject({
      kind: "generic",
      summary: "Session recovery failed",
      requestLocal: true,
    });
  });

  it("keeps the persisted crash card authoritative even when automatic resume also fails", () => {
    const { result } = recoveryFor({
      ...session,
      state: "FAILED",
      error_message: "Agent process exited",
    });
    expect(result.current?.model?.summary).toBe("Agent process exited");
    expect(result.current?.model?.requestLocal).not.toBe(true);
  });

  it("uses typed Restart guidance while its durable recovery projection is delayed", () => {
    const { result } = renderHook(
      () => ({ owner: useAppStoreApi(), context: useSessionComposerRecovery("session") }),
      { wrapper: recoveryWrapper(session) },
    );
    const operation = beginSessionRecoveryGuidanceOperation(
      result.current.owner,
      sessionRecoveryGuidanceIdentityFromSession("task", session),
    );

    act(() => {
      publishSessionRecoveryGuidance(
        result.current.owner,
        operation,
        new WebSocketRequestError("Saved history is available", "CONFLICT", {
          kind: "session_restore_required",
          recovery_action: "continue_from_history",
          reason: "unresolved_durable_work",
          session_id: "session",
          generation: 2,
        }),
      );
    });

    expect(result.current.context?.model).toMatchObject({
      kind: "session_restore_required",
      requestLocal: true,
    });
  });
});

it("restores the composer after external native recovery and fences the late failure", () => {
  mockLaunchErrorContext.value = null;
  let currentSession = deliverySession("uncertain", [recoveryBlock]);
  const { result, rerender } = renderHook(
    () => ({ owner: useAppStoreApi(), context: useSessionComposerRecovery("session") }),
    { wrapper: recoveryWrapperFor(() => currentSession) },
  );
  const owner = result.current.owner;
  const identity = sessionRecoveryGuidanceIdentityFromSession("task", currentSession);
  const operation = beginSessionRecoveryGuidanceOperation(owner, identity);
  const error = new WebSocketRequestError("Saved history is available", "CONFLICT", {
    kind: "session_restore_required",
    recovery_action: "continue_from_history",
    reason: recoveryBlock.reason,
    session_id: "session",
    generation: recoveryBlock.expected_generation,
    block_id: recoveryBlock.id,
    incarnation_id: recoveryBlock.incarnation_id,
  });

  act(() => {
    publishSessionRecoveryGuidance(owner, operation, error, identity);
  });
  expect(result.current.context?.model?.kind).toBe("session_restore_required");

  currentSession = deliverySession("restored", []);
  rerender();

  expect(result.current.context?.model).toBeNull();
  expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();
  let lateFailureAccepted = true;
  act(() => {
    lateFailureAccepted = publishSessionRecoveryGuidance(owner, operation, error, identity);
  });
  expect(lateFailureAccepted).toBe(false);
  expect(result.current.context?.model).toBeNull();
});

it("clears stale local recovery guidance after committed delivery restoration", () => {
  const currentSession = {
    ...deliverySession("restored", []),
    state: "FAILED" as const,
    error_message: "The interrupted prompt failed",
    metadata: {
      ...deliverySession("restored", []).metadata,
      last_agent_error: {
        scope: "session",
        message: "The interrupted prompt failed",
        delivery_submission_id: "submission-1",
      },
    },
  };
  const { result } = renderHook(
    () => ({ owner: useAppStoreApi(), context: useSessionComposerRecovery("session") }),
    { wrapper: recoveryWrapperFor(() => currentSession) },
  );

  expect(result.current.context?.model).toBeNull();
});

it("preserves a new session error after an older delivery incident was restored", () => {
  let currentSession = deliverySession("restored", []);
  mockLaunchErrorContext.value = null;
  const { result, rerender } = renderHook(() => useSessionComposerRecovery("session"), {
    wrapper: recoveryWrapperFor(() => currentSession),
  });
  expect(result.current?.model).toBeNull();

  currentSession = {
    ...currentSession,
    state: "FAILED",
    error_message: "A later provider failure",
    metadata: {
      ...currentSession.metadata,
      last_agent_error: {
        scope: "session",
        message: "A later provider failure",
        delivery_submission_id: "submission-2",
      },
    },
  };
  mockLaunchErrorContext.value = {
    automaticRecovery: {
      requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 2 },
      resumptionState: "error",
      error: "A later recovery request failed",
    },
  };
  rerender();

  expect(result.current?.model).toMatchObject({
    kind: "generic",
    summary: "A later provider failure",
  });
});

it("preserves a new request-local failure after committed restoration", () => {
  mockLaunchErrorContext.value = null;
  const currentSession = deliverySession("restored", []);
  const { result, rerender } = renderHook(() => useSessionComposerRecovery("session"), {
    wrapper: recoveryWrapper(currentSession),
  });
  expect(result.current?.model).toBeNull();

  mockLaunchErrorContext.value = {
    automaticRecovery: {
      requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 2 },
      resumptionState: "error",
      error: "The new recovery request failed",
    },
  };
  rerender();

  expect(result.current?.model).toMatchObject({
    kind: "generic",
    summary: "The new recovery request failed",
    requestLocal: true,
  });
});

it("keeps recovery guidance when another recovery block remains open", () => {
  const currentSession = {
    ...deliverySession("restored", [recoveryBlock]),
    state: "WAITING_FOR_INPUT" as const,
    session_recovery_blocks: [recoveryBlock],
  };
  const { result } = renderHook(() => useSessionComposerRecovery("session"), {
    wrapper: recoveryWrapper(currentSession),
  });

  expect(result.current?.model).toMatchObject({ kind: "generic", requestLocal: true });
});

it("does not place an authoritatively usable session behind unprojected recovery guidance", () => {
  mockLaunchErrorContext.value = null;
  let currentSession = deliverySession("uncertain", undefined);
  const { result, rerender } = renderHook(
    () => ({ owner: useAppStoreApi(), context: useSessionComposerRecovery("session") }),
    { wrapper: recoveryWrapperFor(() => currentSession) },
  );
  const owner = result.current.owner;
  const identity = sessionRecoveryGuidanceIdentityFromSession("task", currentSession);
  const operation = beginSessionRecoveryGuidanceOperation(owner, identity);
  const error = new WebSocketRequestError("Saved history is available", "CONFLICT", {
    kind: "session_restore_required",
    recovery_action: "continue_from_history",
    reason: "unresolved_durable_work",
    session_id: "session",
    generation: 2,
    block_id: recoveryBlock.id,
    incarnation_id: recoveryBlock.incarnation_id,
  });

  act(() => {
    publishSessionRecoveryGuidance(owner, operation, error, identity);
  });
  expect(result.current.context?.model?.kind).toBe("session_restore_required");

  currentSession = {
    ...deliverySession("uncertain", undefined),
    state: "RUNNING",
  };
  rerender();

  expect(result.current.context?.model).toBeNull();
  expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();
  let lateFailureAccepted = true;
  act(() => {
    lateFailureAccepted = publishSessionRecoveryGuidance(owner, operation, error, identity);
  });
  expect(lateFailureAccepted).toBe(false);
  expect(result.current.context?.model).toBeNull();
});

it("does not carry a pending recovery action into another session", () => {
  mockLaunchErrorContext.value = null;
  const sessionA = { ...session, id: sessionId("session-a") };
  const sessionB = { ...session, id: sessionId("session-b"), state: "STARTING" as const };
  let currentSession = sessionA;
  const releasePending = claimSessionRecovery(`task\u0000${sessionA.id}`, "continue_from_history");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider
      initialState={{
        taskSessions: { items: { [sessionA.id]: sessionA, [sessionB.id]: sessionB } },
      }}
    >
      <SessionRecoveryProvider session={currentSession} messages={[]} taskId="task" enabled>
        {children}
      </SessionRecoveryProvider>
    </StateProvider>
  );
  const { result, rerender } = renderHook(() => useSessionComposerRecovery(currentSession.id), {
    wrapper,
  });

  expect(result.current?.pending).toBe("continue_from_history");

  currentSession = sessionB;
  mockLaunchErrorContext.value = {
    automaticRecovery: {
      requestIdentity: { taskId: "task", sessionId: sessionB.id, generation: 2, attemptId: 1 },
      resumptionState: "error",
      error: "Session recovery failed",
    },
  };
  rerender();

  expect(result.current?.model?.requestLocal).toBe(true);
  expect(result.current?.pending).toBe("resume");
  releasePending?.();
});
