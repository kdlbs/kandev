import type { StateCreator } from "zustand";
import type { AutomationsSlice, AutomationsSliceState } from "./types";

export const defaultAutomationsState: AutomationsSliceState = {
  automations: { items: [], loaded: false, loading: false },
  automationRuns: { byAutomationId: {}, loading: {} },
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
      }),
    updateAutomation: (automation) =>
      set((draft) => {
        const idx = draft.automations.items.findIndex((a) => a.id === automation.id);
        if (idx >= 0) {
          draft.automations.items[idx] = automation;
        }
      }),
    removeAutomation: (id) =>
      set((draft) => {
        draft.automations.items = draft.automations.items.filter((a) => a.id !== id);
      }),
  };
}

function createRunsActions(
  set: ImmerSet,
): Pick<
  AutomationsSlice,
  "setAutomationRuns" | "setAutomationRunsLoading" | "removeAutomationRun" | "clearAutomationRuns"
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
  };
}

export const createAutomationsSlice: StateCreator<
  AutomationsSlice,
  [["zustand/immer", never]],
  [],
  AutomationsSlice
> = (set, _get, _api) => ({
  ...defaultAutomationsState,
  ...createAutomationsActions(set),
  ...createRunsActions(set),
});
