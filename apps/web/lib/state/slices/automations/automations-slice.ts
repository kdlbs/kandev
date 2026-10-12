import type { StateCreator } from "zustand";
import type { AutomationsSlice, AutomationsSliceState } from "./types";

export const defaultAutomationsState: AutomationsSliceState = {
  automations: { byWorkspace: {}, items: [], loaded: false, loading: false, triggerTypes: {} },
  automationRuns: { byAutomationId: {}, loading: {}, mutationEpoch: {}, deleting: {} },
};

type ImmerSet = Parameters<
  StateCreator<AutomationsSlice, [["zustand/immer", never]], [], AutomationsSlice>
>[0];

function createAutomationsActions(
  set: ImmerSet,
): Pick<
  AutomationsSlice,
  | "setAutomations"
  | "setAutomationsLoading"
  | "addAutomation"
  | "updateAutomation"
  | "removeAutomation"
> {
  return {
    setAutomations: (items) =>
      set((draft) => {
        draft.automations.items = items;
        draft.automations.loaded = true;
      }),
    setAutomationsLoading: (loading) =>
      set((draft) => {
        draft.automations.loading = loading;
      }),
    addAutomation: (automation) =>
      set((draft) => {
        draft.automations.items.unshift(automation);
        const list = draft.automations.byWorkspace?.[automation.workspace_id];
        if (list?.loaded) list.items.unshift(automation);
      }),
    updateAutomation: (automation) =>
      set((draft) => {
        const idx = draft.automations.items.findIndex((a) => a.id === automation.id);
        if (idx >= 0) {
          draft.automations.items[idx] = automation;
        }
        const list = draft.automations.byWorkspace?.[automation.workspace_id];
        const scopedIndex = list?.items.findIndex((item) => item.id === automation.id) ?? -1;
        if (list && scopedIndex >= 0) list.items[scopedIndex] = automation;
      }),
    removeAutomation: (id) =>
      set((draft) => {
        draft.automations.items = draft.automations.items.filter((a) => a.id !== id);
        for (const list of Object.values(draft.automations.byWorkspace ?? {})) {
          list.items = list.items.filter((item) => item.id !== id);
        }
      }),
  };
}

function createListActions(
  set: ImmerSet,
  get: () => AutomationsSlice,
): Pick<AutomationsSlice, "beginAutomationsList" | "finishAutomationsList"> {
  return {
    beginAutomationsList: (workspaceId, refresh = false) => {
      const current = get().automations.byWorkspace?.[workspaceId];
      if (!refresh && (current?.loaded || current?.loading)) return null;
      const generation = (current?.generation ?? 0) + 1;
      set((draft) => {
        const lists = (draft.automations.byWorkspace ??= {});
        lists[workspaceId] = {
          items: current?.items ?? [],
          loaded: current?.loaded ?? false,
          loading: true,
          generation,
        };
        draft.automations.loading = true;
      });
      return generation;
    },
    finishAutomationsList: (workspaceId, generation, items) =>
      set((draft) => {
        const list = draft.automations.byWorkspace?.[workspaceId];
        if (!list || list.generation !== generation) return;
        if (items !== undefined) {
          list.items = items.filter((item) => item.workspace_id === workspaceId);
        } else if (!list.loaded) {
          list.items = [];
        }
        list.loaded = true;
        list.loading = false;
        draft.automations.items = [...list.items];
        draft.automations.loaded = true;
        draft.automations.loading = Object.values(draft.automations.byWorkspace ?? {}).some(
          (entry) => entry.loading,
        );
      }),
  };
}

function createRunsActions(
  set: ImmerSet,
  get: () => AutomationsSlice,
): Pick<
  AutomationsSlice,
  | "setAutomationRuns"
  | "setAutomationRunsLoading"
  | "removeAutomationRun"
  | "clearAutomationRuns"
  | "restoreAutomationRun"
  | "beginAutomationRunDelete"
  | "endAutomationRunDelete"
  | "advanceAutomationRunEpoch"
> {
  return {
    setAutomationRuns: (automationId, runs) =>
      set((draft) => {
        draft.automationRuns.byAutomationId[automationId] = runs;
      }),
    setAutomationRunsLoading: (automationId, loading) =>
      set((draft) => {
        draft.automationRuns.loading[automationId] = loading;
      }),
    removeAutomationRun: (automationId, runId) =>
      set((draft) => {
        const runs = draft.automationRuns.byAutomationId[automationId];
        if (runs) {
          draft.automationRuns.byAutomationId[automationId] = runs.filter((r) => r.id !== runId);
        }
      }),
    clearAutomationRuns: (automationId) =>
      set((draft) => {
        draft.automationRuns.byAutomationId[automationId] = [];
      }),
    // Re-inserts a single run that was optimistically removed but whose
    // deletion could not be confirmed AND whose recovery refresh also
    // failed — the last-resort fallback so the UI never silently drifts
    // from a known state. Idempotent (no-op if already present) and keeps
    // the list sorted by created_at desc, matching the server's ordering.
    restoreAutomationRun: (automationId, run) =>
      set((draft) => {
        const runs = draft.automationRuns.byAutomationId[automationId] ?? [];
        if (runs.some((r) => r.id === run.id)) return;
        const insertAt = runs.findIndex((r) => r.created_at < run.created_at);
        const next = runs.slice();
        if (insertAt === -1) {
          next.push(run);
        } else {
          next.splice(insertAt, 0, run);
        }
        draft.automationRuns.byAutomationId[automationId] = next;
      }),
    beginAutomationRunDelete: (automationId) => {
      // `?? false` treats the never-started automation (absent key) as idle.
      if ((get().automationRuns.deleting[automationId] ?? false) !== false) return null;
      const next = (get().automationRuns.mutationEpoch[automationId] ?? 0) + 1;
      set((draft) => {
        draft.automationRuns.mutationEpoch[automationId] = next;
        draft.automationRuns.deleting[automationId] = next;
      });
      return next;
    },
    endAutomationRunDelete: (automationId, generation) =>
      set((draft) => {
        if (draft.automationRuns.deleting[automationId] === generation) {
          draft.automationRuns.deleting[automationId] = false;
        }
      }),
    advanceAutomationRunEpoch: (automationId) =>
      set((draft) => {
        draft.automationRuns.mutationEpoch[automationId] =
          (draft.automationRuns.mutationEpoch[automationId] ?? 0) + 1;
      }),
  };
}

export const createAutomationsSlice: StateCreator<
  AutomationsSlice,
  [["zustand/immer", never]],
  [],
  AutomationsSlice
> = (set, get, _api) => ({
  ...defaultAutomationsState,
  ...createAutomationsActions(set),
  ...createListActions(set, get),
  ...createRunsActions(set, get),
  beginTriggerTypes: (workspaceId) => {
    const current = get().automations.triggerTypes[workspaceId];
    if (current?.loading) return null;
    const generation = (current?.generation ?? 0) + 1;
    set((draft) => {
      draft.automations.triggerTypes[workspaceId] = { items: [], loading: true, generation };
    });
    return generation;
  },
  finishTriggerTypes: (workspaceId, generation, items) =>
    set((draft) => {
      if (draft.automations.triggerTypes[workspaceId]?.generation === generation) {
        draft.automations.triggerTypes[workspaceId] = { items, loading: false, generation };
      }
    }),
});
