import type { WorkloadRunObservation } from "@/lib/types/background-work";
import type { BackgroundOutputWindow, BackgroundWorkState, SessionRuntimeSlice } from "./types";
import type { ImmerSet } from "./session-runtime-slice";

function newWindow(state: BackgroundWorkState, runId: string | undefined): BackgroundOutputWindow {
  return { runId, generation: state.nextOutputWindowGeneration++, evicted: 0 };
}

export function reconcileOutputWindows(
  state: BackgroundWorkState,
  sessionId: string,
  previous: WorkloadRunObservation[],
  current: WorkloadRunObservation[],
) {
  const oldWorkloads = new Map(previous.map((work) => [work.work_id, work]));
  const oldWindows = state.outputWindowsBySessionId[sessionId] ?? {};
  const windows: Record<string, BackgroundOutputWindow> = {};
  for (const work of current) {
    const old = oldWorkloads.get(work.work_id);
    const window = oldWindows[work.work_id];
    windows[work.work_id] =
      old && window && old.run_id === work.run_id && (old.output || "") === (work.output || "")
        ? window
        : newWindow(state, work.run_id);
  }
  state.outputWindowsBySessionId[sessionId] = windows;
}

export function recordOutputEviction(
  state: BackgroundWorkState,
  sessionId: string,
  work: WorkloadRunObservation,
  count: number,
) {
  const windows = (state.outputWindowsBySessionId[sessionId] ??= {});
  const window = (windows[work.work_id] ??= newWindow(state, work.run_id));
  window.evicted += count;
}

export function buildBackgroundOutputActions(set: ImmerSet) {
  return {
    appendBackgroundWorkloadOutput: (
      sessionId: string,
      chunk: Parameters<SessionRuntimeSlice["appendBackgroundWorkloadOutput"]>[1],
    ) =>
      set((draft) => {
        const list = draft.backgroundWork.workloadsBySessionId[sessionId] ?? [];
        const workload = list.find((w) => w.work_id === chunk.work_id);
        if (workload && chunk.chunk) {
          const currentOutput = workload.output || "";
          const newOutput = currentOutput + chunk.chunk;
          const maxLen = 200 * 1024;
          recordOutputEviction(
            draft.backgroundWork,
            sessionId,
            workload,
            Math.max(0, newOutput.length - maxLen),
          );
          if (newOutput.length > maxLen) {
            workload.output = newOutput.slice(newOutput.length - maxLen);
            workload.output_truncated = true;
          } else {
            workload.output = newOutput;
          }
          workload.output_offset = chunk.offset;
          if (chunk.truncated) {
            workload.output_truncated = true;
          }
        }
      }),
    clearBackgroundWork: (sessionId: string) =>
      set((draft) => {
        delete draft.backgroundWork.workloadsBySessionId[sessionId];
        delete draft.backgroundWork.outputWindowsBySessionId[sessionId];
        delete draft.backgroundWork.activeWorkIdBySessionId[sessionId];
        delete draft.backgroundWork.loadingBySessionId[sessionId];
      }),
  };
}
