import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { PrepareProgressPayload, PrepareCompletedPayload } from "@/lib/types/backend";
import type {
  PrepareStepInfo,
  SessionPrepareState,
} from "@/lib/state/slices/session-runtime/types";
import { comparePreparationStartedAt } from "@/lib/prepare/preparation-attempt";

const MCP_STEP_PREFIX = "agent_mcp_";

function mapStep(step: NonNullable<PrepareCompletedPayload["steps"]>[number]): PrepareStepInfo {
  const isMcp = step.kind?.startsWith(MCP_STEP_PREFIX) === true;
  return {
    name: isMcp ? "" : step.name,
    kind: step.kind,
    remotePlatform: step.remote_platform,
    mcpServerId: step.mcp_server_id,
    mcpProvider: step.mcp_provider,
    failureCode: step.failure_code,
    command: isMcp ? undefined : step.command,
    status: step.status,
    output: isMcp ? undefined : step.output,
    error: isMcp ? undefined : step.error,
    warning: isMcp ? undefined : step.warning,
    warningDetail: isMcp ? undefined : step.warning_detail,
    startedAt: step.started_at,
    endedAt: step.ended_at,
  };
}

function isMcpStep(step: PrepareStepInfo): boolean {
  return step.kind?.startsWith(MCP_STEP_PREFIX) === true;
}

function shouldAcceptAttempt(
  current: SessionPrepareState | undefined,
  preparationId?: string,
  preparationStartedAt?: string,
): { accept: boolean; isNewer: boolean } {
  if (!current) return { accept: true, isNewer: true };
  if (!preparationStartedAt) {
    if (current.preparationStartedAt) return { accept: false, isNewer: false };
    if (current.preparationId && preparationId !== current.preparationId) {
      return { accept: false, isNewer: false };
    }
    return { accept: true, isNewer: false };
  }
  if (!current.preparationStartedAt) return { accept: true, isNewer: true };
  const order = comparePreparationStartedAt(preparationStartedAt, current.preparationStartedAt);
  if (order === null) {
    return {
      accept: Boolean(preparationId && preparationId === current.preparationId),
      isNewer: false,
    };
  }
  if (order < 0) return { accept: false, isNewer: false };
  if (order > 0) return { accept: true, isNewer: true };
  return {
    accept: preparationId === current.preparationId,
    isNewer: false,
  };
}

function updateSteps(
  existing: PrepareStepInfo[],
  payload: PrepareProgressPayload,
): PrepareStepInfo[] {
  const steps = [...existing];
  while (steps.length <= payload.step_index) {
    steps.push({ name: "", status: "pending" });
  }
  const isMcp = payload.step_kind?.startsWith(MCP_STEP_PREFIX) === true;
  steps[payload.step_index] = {
    name: isMcp ? "" : payload.step_name,
    kind: payload.step_kind,
    remotePlatform: payload.remote_platform,
    mcpServerId: payload.mcp_server_id,
    mcpProvider: payload.mcp_provider,
    failureCode: payload.failure_code,
    command: isMcp ? undefined : payload.step_command,
    status: payload.status,
    output: isMcp ? undefined : payload.output,
    error: isMcp ? undefined : payload.error,
    warning: isMcp ? undefined : payload.warning,
    warningDetail: isMcp ? undefined : payload.warning_detail,
    startedAt: payload.started_at,
    endedAt: payload.ended_at,
  };
  return steps;
}

function shouldDropStaleProgress(
  current: SessionPrepareState | undefined,
  payload: PrepareProgressPayload,
  isNewer: boolean,
): boolean {
  if (!current) return false;
  const isFinalized = current.status === "completed" || current.status === "failed";
  if (!isFinalized || isNewer) return false;
  const currentId = current.preparationId ?? "";
  const payloadId = payload.preparation_id ?? "";
  return payloadId === currentId;
}

function resolveExistingSteps(
  current: SessionPrepareState | undefined,
  isNewer: boolean,
): PrepareStepInfo[] {
  const steps = current?.steps ?? [];
  return isNewer ? steps.filter((step) => !isMcpStep(step)) : steps;
}

function buildNextPrepareProgress(
  current: SessionPrepareState | undefined,
  payload: PrepareProgressPayload,
): SessionPrepareState | null {
  const attempt = shouldAcceptAttempt(
    current,
    payload.preparation_id,
    payload.preparation_started_at,
  );
  if (!attempt.accept || shouldDropStaleProgress(current, payload, attempt.isNewer)) {
    return null;
  }

  const existingSteps = resolveExistingSteps(current, attempt.isNewer);
  return {
    ...current,
    sessionId: payload.session_id,
    status: "preparing",
    preparationId: payload.preparation_id ?? current?.preparationId,
    preparationStartedAt: payload.preparation_started_at ?? current?.preparationStartedAt,
    steps: updateSteps(existingSteps, payload),
    errorMessage: attempt.isNewer ? undefined : current?.errorMessage,
    durationMs: attempt.isNewer ? undefined : current?.durationMs,
  };
}

export function registerExecutorPrepareHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "executor.prepare.progress": (message) => {
      const payload = message.payload as PrepareProgressPayload;
      store.setState((state) => {
        const current = state.prepareProgress.bySessionId[payload.session_id];
        const next = buildNextPrepareProgress(current, payload);
        if (!next) return state;
        return {
          ...state,
          prepareProgress: {
            ...state.prepareProgress,
            bySessionId: {
              ...state.prepareProgress.bySessionId,
              [payload.session_id]: next,
            },
          },
        };
      });
    },
    "executor.prepare.completed": (message) => {
      const payload = message.payload as PrepareCompletedPayload;
      store.setState((state) => {
        const existing = state.prepareProgress.bySessionId[payload.session_id];
        const attempt = shouldAcceptAttempt(
          existing,
          payload.preparation_id,
          payload.preparation_started_at,
        );
        if (!attempt.accept) return state;
        const fallbackSteps = attempt.isNewer
          ? (existing?.steps ?? []).filter((step) => !isMcpStep(step))
          : existing?.steps;
        const steps = payload.steps?.length ? payload.steps.map(mapStep) : fallbackSteps;

        return {
          ...state,
          prepareProgress: {
            ...state.prepareProgress,
            bySessionId: {
              ...state.prepareProgress.bySessionId,
              [payload.session_id]: {
                ...existing,
                sessionId: payload.session_id,
                status: payload.success ? "completed" : "failed",
                preparationId: payload.preparation_id ?? existing?.preparationId,
                preparationStartedAt:
                  payload.preparation_started_at ?? existing?.preparationStartedAt,
                steps: steps ?? [],
                errorMessage: payload.error_message,
                durationMs: payload.duration_ms,
              },
            },
          },
        };
      });
    },
  };
}
