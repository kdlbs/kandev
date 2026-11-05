import type { SessionRuntimeSlice } from "./types";
import type { ImmerSet } from "./session-runtime-slice";
import { buildBackgroundOutputActions, reconcileOutputWindows } from "./background-output-window";
import {
  applyWorkloadActionAvailability,
  hasCapabilityDeclaration,
} from "@/lib/background-work/capabilities";
import type { WorkloadRunObservation } from "@/lib/types/background-work";

function isTerminalRunState(s?: string) {
  return s === "completed" || s === "failed" || s === "interrupted" || s === "ended";
}

/**
 * Merge a projection's capability declaration. Publications that project the
 * durable row (state changes, retained output) can omit adapter-owned
 * capabilities, so the existing declaration is kept. Dynamic availability is
 * always re-evaluated against the incoming state: a terminal or unknown
 * transition must not leave a previously available control enabled.
 */
function mergeWorkloadCapabilities(
  existing: WorkloadRunObservation,
  incoming: WorkloadRunObservation,
): WorkloadRunObservation["capabilities"] {
  const declaration = hasCapabilityDeclaration(incoming.capabilities)
    ? incoming.capabilities
    : existing.capabilities;
  return applyWorkloadActionAvailability(declaration, incoming.state);
}

function buildBackgroundWorkLoadActions(set: ImmerSet) {
  return {
    setBackgroundWorkloads: (
      sessionId: string,
      workloads: Parameters<SessionRuntimeSlice["setBackgroundWorkloads"]>[1],
    ) =>
      set((draft) => {
        const existingList = draft.backgroundWork.workloadsBySessionId[sessionId] ?? [];
        const existingMap = new Map(existingList.map((w) => [w.work_id, w]));
        const merged = workloads.map((incoming) => {
          const existing = existingMap.get(incoming.work_id);
          if (!existing) {
            return {
              ...incoming,
              capabilities: applyWorkloadActionAvailability(incoming.capabilities, incoming.state),
            };
          }
          if (existing.revision > incoming.revision) {
            return existing;
          }
          if (isTerminalRunState(existing.state) && !isTerminalRunState(incoming.state)) {
            return {
              ...incoming,
              state: existing.state,
              exit_code: existing.exit_code,
              finished_at: existing.finished_at,
              revision: Math.max(incoming.revision, existing.revision),
              capabilities: mergeWorkloadCapabilities(existing, incoming),
            };
          }
          return { ...incoming, capabilities: mergeWorkloadCapabilities(existing, incoming) };
        });
        reconcileOutputWindows(draft.backgroundWork, sessionId, existingList, merged);
        draft.backgroundWork.workloadsBySessionId[sessionId] = merged;
        draft.backgroundWork.loadingBySessionId[sessionId] = false;
      }),
    updateBackgroundWorkload: (
      sessionId: string,
      workload: Parameters<SessionRuntimeSlice["updateBackgroundWorkload"]>[1],
    ) =>
      set((draft) => {
        const list = draft.backgroundWork.workloadsBySessionId[sessionId] ?? [];
        const previous = [...list];
        const idx = list.findIndex((w) => w.work_id === workload.work_id);
        if (idx >= 0) {
          const existing = list[idx]!;
          if (existing.revision > workload.revision) {
            return;
          }
          if (isTerminalRunState(existing.state) && !isTerminalRunState(workload.state)) {
            return;
          }
          list[idx] = {
            ...existing,
            ...workload,
            capabilities: mergeWorkloadCapabilities(existing, workload),
            output: workload.output !== undefined ? workload.output : existing.output,
          };
        } else {
          list.push({
            ...workload,
            capabilities: applyWorkloadActionAvailability(workload.capabilities, workload.state),
          });
        }
        reconcileOutputWindows(draft.backgroundWork, sessionId, previous, list);
        draft.backgroundWork.workloadsBySessionId[sessionId] = list;
      }),
    setActiveBackgroundWorkload: (sessionId: string, workId: string) =>
      set((draft) => {
        draft.backgroundWork.activeWorkIdBySessionId[sessionId] = workId;
      }),
    setBackgroundWorkLoading: (sessionId: string, loading: boolean) =>
      set((draft) => {
        draft.backgroundWork.loadingBySessionId[sessionId] = loading;
      }),
  };
}

/** Background-work store actions. */
export function buildBackgroundWorkActions(set: ImmerSet) {
  return {
    ...buildBackgroundOutputActions(set),
    ...buildBackgroundWorkLoadActions(set),
  };
}
