import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import { hydrateState } from "./hydration/hydrator";
import type { AppState, HydrationState } from "./app-state-types";
import { mergeInitialState } from "./default-state";
import { buildStateOverrides } from "./store-overrides";

import {
  createKanbanSlice,
  createWorkspaceSlice,
  createSettingsSlice,
  createSessionSlice,
  createSessionRuntimeSlice,
  createUISlice,
  createGitHubSlice,
  createGitLabSlice,
  createAzureDevOpsSlice,
  createJiraSlice,
  createLinearSlice,
  createOfficeSlice,
  createFeaturesSlice,
  createAuthSlice,
  createAutomationsSlice,
  createSystemSlice,
  createPluginsSlice,
  createReviewSlice,
  createLspSlice,
} from "./slices";

// Re-export all types from slices for backwards compatibility.
export type { AppState, HydrationState } from "./app-state-types";
export type * from "./store-reexports";

/** Creates the Zustand app store, hydrating from `initialState` and
 * composing every domain slice (kanban, ui, workspace, settings, ...). */
export function createAppStore(initialState?: HydrationState) {
  const merged = mergeInitialState(initialState);

  return createStore<AppState>()(
    immer((set, get, api) => ({
      ...merged,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createKanbanSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createWorkspaceSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSettingsSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSessionSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSessionRuntimeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createGitHubSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createGitLabSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAzureDevOpsSlice(set as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createJiraSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createLinearSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createOfficeSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createFeaturesSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAuthSlice(set as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createSystemSlice(set as any, get as any, api as any),
      setAgentRuntime: (snapshot) =>
        set((draft) => {
          draft.agentRuntime = snapshot;
        }),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createUISlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createAutomationsSlice(set as any, get as any, api as any),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createPluginsSlice(set as any, get as any, api as any),
      // createReviewSlice only needs `set`; passing get/api would be superfluous
      // arguments (CodeQL js/superfluous-trailing-arguments).
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...createReviewSlice(set as any),
      ...createLspSlice(set, get, api),
      // Re-assert merged initial state so caller-supplied values win over slice defaults.
      ...buildStateOverrides(merged),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hydrate: (state, options) => set((draft) => hydrateState(draft as any, state, options)),
    })),
  );
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: HydrationState;
};
