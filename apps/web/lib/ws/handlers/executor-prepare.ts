import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { PrepareProgressPayload, PrepareCompletedPayload } from "@/lib/types/backend";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

export function registerExecutorPrepareHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "executor.prepare.progress": (message) => {
      const payload = message.payload as PrepareProgressPayload;
      store.setState((state) => ({
        ...state,
        prepareProgress: {
          ...state.prepareProgress,
          bySessionId: {
            ...state.prepareProgress.bySessionId,
            [payload.session_id]: {
              ...state.prepareProgress.bySessionId[payload.session_id],
              sessionId: payload.session_id,
              status: "preparing",
              steps: updateSteps(
                state.prepareProgress.bySessionId[payload.session_id]?.steps ?? [],
                payload,
              ),
            },
          },
        },
      }));
    },
    "executor.prepare.completed": (message) => {
      const payload = message.payload as PrepareCompletedPayload;
      store.setState((state) => {
        const existing = state.prepareProgress.bySessionId[payload.session_id];
        // Use steps from the completed payload if available (ensures warnings
        // are present even if progress events were missed due to late subscription).
        const steps = payload.steps?.length
          ? payload.steps.map((s) => ({
              name: s.name,
              status: s.status,
              output: s.output,
              error: s.error,
              warning: s.warning,
              warningDetail: s.warning_detail,
            }))
          : existing?.steps;

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

function updateSteps(
  existing: PrepareStepInfo[],
  payload: PrepareProgressPayload,
): PrepareStepInfo[] {
  const steps = [...existing];
  // Ensure array is big enough
  while (steps.length <= payload.step_index) {
    steps.push({ name: "", status: "pending" });
  }
  steps[payload.step_index] = {
    name: payload.step_name,
    status: payload.status,
    output: payload.output,
    error: payload.error,
    warning: payload.warning,
    warningDetail: payload.warning_detail,
  };
  return steps;
}
