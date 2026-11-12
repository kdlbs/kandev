import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";
import {
  beginSessionRecoveryGuidanceOperation,
  publishSessionRecoveryGuidance,
  selectSessionRecoveryGuidance,
  sessionRecoveryGuidanceIdentityFromSession,
} from "@/lib/session/session-recovery-guidance";
import type { TaskSession } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/client";

type RecoveryAppState = {
  taskSessions: { items: Record<string, TaskSession> };
  kanban: { tasks: { id: string; isFromOffice?: boolean }[] };
  quickChat: { sessions: { kind: string; sessionId: string; taskId?: string }[] };
  agentProfiles: { items: { id: string; agent_name: string; cli_passthrough: boolean }[] };
  repositories: { itemsByWorkspaceId: Record<string, unknown[]> };
  setWorkspaceRecoveryProjection: ReturnType<typeof vi.fn>;
};

const mocks = vi.hoisted(() => ({
  requestSessionRecover: vi.fn(),
  setWorkspaceRecoveryProjection: vi.fn(),
  guidanceOwner: {},
  appState: null as unknown as RecoveryAppState,
}));

vi.mock("@/lib/services/session-recovery-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/services/session-recovery-service")>()),
  requestSessionRecover: mocks.requestSessionRecover,
}));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => mocks.guidanceOwner,
  useOptionalAppStoreApi: () => mocks.guidanceOwner,
  useAppStore: (selector: (state: never) => unknown) => selector(mocks.appState as never),
}));

const TASK_ID = "guidance-task";
const SESSION_ID = "guidance-session";
const BLOCK_ID = "block-1";
const INCARNATION_ID = "incarnation-1";
const SUBMISSION_ID = "prompt:message-1";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.guidanceOwner = {};
});

it("retires guidance when same-generation recovery projects before success", async () => {
  const session = deliverySession("FAILED", "uncertain", 1, [openBlock()]);
  mocks.appState = recoveryAppState(session);
  const owner = mocks.guidanceOwner;
  const identity = sessionRecoveryGuidanceIdentityFromSession(TASK_ID, session);
  const oldFailureOperation = beginSessionRecoveryGuidanceOperation(owner, identity);
  const oldFailure = restoreRequiredError();
  expect(publishSessionRecoveryGuidance(owner, oldFailureOperation, oldFailure)).toBe(true);

  const deferred = Promise.withResolvers<void>();
  mocks.requestSessionRecover.mockReturnValueOnce(deferred.promise);
  const { result, rerender } = renderHook(() =>
    useSessionRecoveryActions({ taskId: TASK_ID, sessionId: SESSION_ID }),
  );

  let response!: Promise<boolean>;
  act(() => {
    response = result.current.handleRecover("resume");
  });
  expect(result.current.continuationDetails).not.toBeNull();

  const restoredSession = deliverySession("RUNNING", "restored", 2, []);
  mocks.appState = recoveryAppState(restoredSession);
  rerender();
  expect(result.current.continuationDetails).toBeNull();
  expect(selectSessionRecoveryGuidance(owner, identity)).toBeNull();

  await act(async () => {
    deferred.resolve();
    await response;
  });
  expect(result.current.continuationDetails).toBeNull();
  expect(publishSessionRecoveryGuidance(owner, oldFailureOperation, oldFailure)).toBe(false);
});

function deliverySession(
  state: "FAILED" | "RUNNING",
  phase: "uncertain" | "restored",
  revision: number,
  blocks: TaskSession["session_recovery_blocks"],
): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state,
    metadata: {
      agent_delivery_recovery: {
        phase,
        revision,
        session_id: SESSION_ID,
        agent_execution_id: "execution-1",
        submission_id: SUBMISSION_ID,
        stream_id: "stream-1",
        incarnation_id: INCARNATION_ID,
        harness_generation: 7,
        prompt_generation: 1,
      },
    },
    session_recovery_blocks: blocks,
  } as unknown as TaskSession;
}

function openBlock(): NonNullable<TaskSession["session_recovery_blocks"]>[number] {
  return {
    id: BLOCK_ID,
    incarnation_id: INCARNATION_ID,
    expected_generation: 7,
    reason: "unresolved_durable_work",
    consumer_reference: "agent_delivery",
    delivery_submission_id: SUBMISSION_ID,
    updated_at: "2026-11-11T00:00:00Z",
  };
}

function restoreRequiredError() {
  return new WebSocketRequestError("Saved history is available", "CONFLICT", {
    kind: "session_restore_required",
    recovery_action: "continue_from_history",
    reason: "unresolved_durable_work",
    session_id: SESSION_ID,
    generation: 7,
    block_id: BLOCK_ID,
    incarnation_id: INCARNATION_ID,
  });
}

function recoveryAppState(session: TaskSession) {
  return {
    taskSessions: { items: { [SESSION_ID]: session } },
    kanban: { tasks: [{ id: TASK_ID, isFromOffice: false }] },
    quickChat: { sessions: [] },
    agentProfiles: { items: [] },
    repositories: { itemsByWorkspaceId: {} },
    setWorkspaceRecoveryProjection: mocks.setWorkspaceRecoveryProjection,
  };
}
