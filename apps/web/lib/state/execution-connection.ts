import type { AppState } from "./app-state-types";
import type { SliceSet } from "./slice-creator";
import type { TaskSession } from "@/lib/types/http";

export function buildExecutionConnectionActions(
  set: SliceSet<AppState>,
): Pick<AppState, "setConnectionStatus"> {
  return {
    setConnectionStatus: (status, error) =>
      set((draft) => {
        if (status !== "connected" || draft.connection.status !== "connected") {
          draft.connection.epoch = (draft.connection.epoch ?? 0) + 1;
          invalidateExecutionObservations(draft);
        }
        draft.connection.status = status;
        draft.connection.error = error ?? null;
      }),
  };
}

/** Invalidate live claims while retaining attempt identity and evidence history. */
function invalidateExecutionObservations(
  state: Pick<AppState, "taskSessions" | "taskSessionsByTask">,
): void {
  for (const session of Object.values(state.taskSessions.items)) invalidate(session);
  for (const sessions of Object.values(state.taskSessionsByTask.itemsByTaskId)) {
    for (const session of sessions) invalidate(session);
  }
}

function invalidate(session: TaskSession): void {
  const value = session.execution_observation;
  if (!value || value.status === "stopped" || value.status === "failed") return;
  session.execution_observation = {
    ...value,
    status: "unknown",
    provider_health: "unknown",
    reason: "evidence_unavailable",
    valid_until: undefined,
  };
}
