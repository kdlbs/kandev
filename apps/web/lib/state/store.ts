import { invalidateExecutorCatalogue } from "@/lib/api/executor-catalogue";
import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import { hydrateState } from "./hydration/hydrator";
import type { AppState, HydrationState } from "./app-state-types";
import { mergeInitialState } from "./default-state";
import { buildStateOverrides } from "./store-overrides";
import { getQuickChatSelectionIdentity } from "@/lib/quick-chat/selection-storage";
import { newerAgentRuntimeSnapshot } from "@/lib/types/agent-runtime";
import { createTaskOverviewSlice } from "./slices/task-overview";
import { withTaskOverviewNormalization } from "./slices/task-overview-normalize";
import { observeSettingsAgentRemovals } from "./settings-agent-removals";

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
  createCoordinatorsSlice,
  createSystemSlice,
  createPluginsSlice,
  createReviewSlice,
  createNeedsYouInboxSlice,
  createFailedInboxSlice,
  createPreviewFeedbackSlice,
  createInboxHistorySlice,
} from "./slices";

// Re-export all types from slices for backwards compatibility.
export type { AppState, HydrationState } from "./app-state-types";
export type * from "./store-reexports";

/** Creates the Zustand app store, hydrating from `initialState` and
 * composing every domain slice (kanban, ui, workspace, settings, ...). */
export function createAppStore(initialState?: HydrationState) {
  const merged = mergeInitialState(initialState);

  const store = createStore<AppState>()(
    withTaskOverviewNormalization(
      immer((set, get, api) => ({
        ...merged,
        ...createTaskOverviewSlice(set, get, api),
        ...createKanbanSlice(set, get),
        ...createWorkspaceSlice(set, get),
        ...createSettingsSlice(set, get),
        ...createSessionSlice(set, get),
        ...createSessionRuntimeSlice(set, get),
        ...createGitHubSlice(set, get),
        ...createGitLabSlice(set, get),
        ...createAzureDevOpsSlice(set),
        ...createJiraSlice(set, get),
        ...createLinearSlice(set, get),
        ...createOfficeSlice(set, get),
        ...createFeaturesSlice(set),
        ...createAuthSlice(set),
        ...createSystemSlice(set, get),
        setAgentRuntime: (snapshot) =>
          set((draft) => {
            draft.agentRuntime = newerAgentRuntimeSnapshot(draft.agentRuntime, snapshot);
          }),
        ...createUISlice(set, get),
        ...createAutomationsSlice(set, get),
        ...createCoordinatorsSlice(set),
        ...createPluginsSlice(set, get),
        ...createReviewSlice(set),
        ...createNeedsYouInboxSlice(set),
        ...createFailedInboxSlice(set, get),
        ...createPreviewFeedbackSlice(set),
        ...createInboxHistorySlice(set),
        // Re-assert merged initial state so caller-supplied values win over slice defaults.
        ...buildStateOverrides(merged),
        hydrate: (state, options) => set((draft) => hydrateState(draft, state, options)),
      })),
    ),
  );
  observeSettingsAgentRemovals(store);
  let previousAuth = store.getState().auth;
  const syncQuickChatSelectionIdentity = (auth: AppState["auth"]) => {
    const identity = getQuickChatSelectionIdentity(auth);
    if (store.getState().quickChat.selectionStorageIdentity !== identity) {
      store.getState().setQuickChatSelectionIdentity(identity);
    }
  };
  syncQuickChatSelectionIdentity(previousAuth);
  store.subscribe((state) => {
    if (state.auth === previousAuth) return;
    const changedUser = state.auth.user?.id !== previousAuth.user?.id;
    const loggedOut = previousAuth.authenticated && !state.auth.authenticated;
    const changedAuthority =
      state.auth.user?.role !== previousAuth.user?.role ||
      state.auth.user?.status !== previousAuth.user?.status ||
      state.auth.mode !== previousAuth.mode;
    if (changedUser || loggedOut || changedAuthority) {
      invalidateExecutorCatalogue();
      previousAuth = state.auth;
      store.getState().setExecutors([]);
      store.getState().setSettingsData({ executorsLoaded: false });
    }
    previousAuth = state.auth;
    syncQuickChatSelectionIdentity(state.auth);
  });
  return store;
}

export type StoreProviderProps = {
  children: React.ReactNode;
  initialState?: HydrationState;
};
