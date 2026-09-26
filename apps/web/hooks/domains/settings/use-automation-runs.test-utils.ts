import type { AutomationRun } from "@/lib/types/automation";

export const AUTOMATION_ID = "auto-1";
export const WORKSPACE_ID = "ws-1";

export type MockRunsState = {
  automationRuns: {
    byAutomationId: Record<string, AutomationRun[]>;
    loading: Record<string, boolean>;
    mutationEpoch: Record<string, number>;
    deleting: Record<string, number | false>;
  };
};

function freshState(): MockRunsState {
  return { automationRuns: { byAutomationId: {}, loading: {}, mutationEpoch: {}, deleting: {} } };
}

/**
 * Shared mutable mock of the app store's automation-runs surface, mirroring
 * `createAutomationsSlice`. Test files reset it in `beforeEach`; the
 * `vi.mock("@/components/state-provider")` factories read `runsStore.get()`.
 * `mockStoreApi` is a single stable object (like the production context
 * store) so the hook's per-store request registry keys stay consistent.
 */
export const runsStore = {
  state: freshState(),
  reset() {
    this.state = freshState();
  },
  get() {
    return { ...this.state, ...actions };
  },
};

export const mockStoreApi = {
  getState: () => runsStore.get(),
};

function setRunsInternal(automationId: string, runs: AutomationRun[]) {
  runsStore.state = {
    automationRuns: {
      ...runsStore.state.automationRuns,
      byAutomationId: { ...runsStore.state.automationRuns.byAutomationId, [automationId]: runs },
    },
  };
}

/** Seed the mock store's run list for an automation. */
export function setRuns(automationId: string, runs: AutomationRun[]) {
  setRunsInternal(automationId, runs);
}

export const actions = {
  setAutomationRuns: setRuns,
  setAutomationRunsLoading: (automationId: string, loading: boolean) => {
    runsStore.state = {
      automationRuns: {
        ...runsStore.state.automationRuns,
        loading: { ...runsStore.state.automationRuns.loading, [automationId]: loading },
      },
    };
  },
  removeAutomationRun: (automationId: string, runId: string) => {
    const runs = runsStore.state.automationRuns.byAutomationId[automationId] ?? [];
    setRunsInternal(
      automationId,
      runs.filter((r) => r.id !== runId),
    );
  },
  clearAutomationRuns: (automationId: string) => setRunsInternal(automationId, []),
  restoreAutomationRun: (automationId: string, run: AutomationRun) => {
    const runs = runsStore.state.automationRuns.byAutomationId[automationId] ?? [];
    if (runs.some((r) => r.id === run.id)) return;
    // Mirrors the production slice: insert keeping newest-first order.
    const insertAt = runs.findIndex((r) => r.created_at < run.created_at);
    const next = runs.slice();
    if (insertAt === -1) {
      next.push(run);
    } else {
      next.splice(insertAt, 0, run);
    }
    setRunsInternal(automationId, next);
  },
  beginAutomationRunDelete: (automationId: string) => {
    if ((runsStore.state.automationRuns.deleting[automationId] ?? false) !== false) return null;
    const next = (runsStore.state.automationRuns.mutationEpoch[automationId] ?? 0) + 1;
    runsStore.state = {
      automationRuns: {
        ...runsStore.state.automationRuns,
        mutationEpoch: {
          ...runsStore.state.automationRuns.mutationEpoch,
          [automationId]: next,
        },
        deleting: { ...runsStore.state.automationRuns.deleting, [automationId]: next },
      },
    };
    return next;
  },
  endAutomationRunDelete: (automationId: string, generation: number) => {
    if (runsStore.state.automationRuns.deleting[automationId] === generation) {
      runsStore.state = {
        automationRuns: {
          ...runsStore.state.automationRuns,
          deleting: { ...runsStore.state.automationRuns.deleting, [automationId]: false },
        },
      };
    }
  },
  advanceAutomationRunEpoch: (automationId: string) => {
    runsStore.state = {
      automationRuns: {
        ...runsStore.state.automationRuns,
        mutationEpoch: {
          ...runsStore.state.automationRuns.mutationEpoch,
          [automationId]: (runsStore.state.automationRuns.mutationEpoch[automationId] ?? 0) + 1,
        },
      },
    };
  },
};

/** A promise plus its resolve/reject functions, for controlling when a mocked
 * async call settles relative to other events in a test. */
export function deferred<T>() {
  const { promise, resolve, reject } = Promise.withResolvers<T>();
  return { promise, resolve, reject };
}

export function mkRun(id: string): AutomationRun {
  return {
    id,
    automation_id: AUTOMATION_ID,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "",
    status: "skipped",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
  };
}
