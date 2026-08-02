"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";
import type {
  AzureDevOpsBrowseMode,
  AzureDevOpsFiltersState,
} from "@/components/azure-devops/azure-devops-filters";
import type { AzureDevOpsScopeSelection } from "@/components/azure-devops/azure-devops-scope-bar";

export type AzureDevOpsBoardPreference = {
  teamId?: string;
  boardId?: string;
  focusedColumnId?: string;
};

export type AzureDevOpsBrowsePreference = {
  mode?: AzureDevOpsBrowseMode;
  selection?: AzureDevOpsScopeSelection;
  filters?: Partial<AzureDevOpsFiltersState>;
  board?: AzureDevOpsBoardPreference;
};

export type AzureDevOpsBrowsePreferences = Record<string, AzureDevOpsBrowsePreference>;

type PagePreferenceState = {
  mode: AzureDevOpsBrowseMode;
  setMode: Dispatch<SetStateAction<AzureDevOpsBrowseMode>>;
  filters: AzureDevOpsFiltersState;
  replaceFilters: (filters: AzureDevOpsFiltersState) => void;
  selection: AzureDevOpsScopeSelection;
  setSelection: Dispatch<SetStateAction<AzureDevOpsScopeSelection>>;
  defaultFilters: AzureDevOpsFiltersState;
};

export function readAzureDevOpsBrowsePreferences(value: unknown): AzureDevOpsBrowsePreferences {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return value as AzureDevOpsBrowsePreferences;
}

const syncPreferences = createQueuedUserSettingsSync<AzureDevOpsBrowsePreferences>((value) => ({
  azure_devops_browse_preferences: value,
}));

export function useAzureDevOpsPreferences(workspaceId: string | undefined) {
  const stored = useAppStore((state) => state.userSettings.azureDevOpsBrowsePreferences);
  const loaded = useAppStore((state) => state.userSettings.loaded);
  const store = useAppStoreApi();
  const [optimistic, setOptimistic] = useState<AzureDevOpsBrowsePreferences | null>(null);
  const revision = useRef(0);
  const preferences = useMemo(
    () => optimistic ?? readAzureDevOpsBrowsePreferences(stored),
    [optimistic, stored],
  );
  const preference = workspaceId ? preferences[workspaceId] : undefined;

  const save = useCallback(
    (next: AzureDevOpsBrowsePreference) => {
      if (!workspaceId) return;
      const nextPreferences = { ...preferences, [workspaceId]: next };
      const currentRevision = ++revision.current;
      setOptimistic(nextPreferences);
      void syncPreferences(nextPreferences)
        .then(() => {
          if (currentRevision !== revision.current) return;
          const state = store.getState();
          state.setUserSettings({
            ...state.userSettings,
            azureDevOpsBrowsePreferences: nextPreferences,
          });
          setOptimistic(null);
        })
        .catch(() => {
          if (currentRevision === revision.current) setOptimistic(null);
        });
    },
    [preferences, store, workspaceId],
  );

  return { loaded, preference, save };
}

export function useAzureDevOpsPagePreferences(
  workspaceId: string | undefined,
  state: PagePreferenceState,
) {
  const { loaded, preference, save } = useAzureDevOpsPreferences(workspaceId);
  const [hydrated, setHydrated] = useState(false);
  const [board, setBoard] = useState<AzureDevOpsBoardPreference>({});
  const initialized = useRef(false);
  const persisted = useRef("");

  useEffect(() => {
    if (!loaded || initialized.current) return;
    if (preference?.mode) state.setMode(preference.mode);
    if (preference?.filters)
      state.replaceFilters({ ...state.defaultFilters, ...preference.filters });
    if (preference?.selection) state.setSelection(preference.selection);
    setBoard(preference?.board ?? {});
    persisted.current = JSON.stringify(preference ?? {});
    initialized.current = true;
    setHydrated(true);
  }, [loaded, preference, state]);

  useEffect(() => {
    if (!hydrated) return;
    const next = { mode: state.mode, selection: state.selection, filters: state.filters, board };
    const serialized = JSON.stringify(next);
    if (serialized === persisted.current) return;
    persisted.current = serialized;
    save(next);
  }, [board, hydrated, save, state]);

  return { hydrated, board, setBoard };
}
