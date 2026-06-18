"use client";

import { useCallback, useEffect, useState } from "react";
import { DEFAULT_FILTERS, type FilterState } from "./filter-model";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  hasUserSettingsSyncFailure,
  setUserSettingsSyncFailure,
} from "@/lib/user-settings-sync-failure";

export type SavedView = {
  id: string;
  name: string;
  filters: FilterState;
  // customJql is set when the user saved the view while the raw JQL editor was
  // overriding the structured filters. Restoring such a view re-applies the
  // exact JQL string instead of recomposing it from `filters`.
  customJql?: string | null;
  builtin?: boolean;
};

const STORAGE_KEY = "kandev:jira:saved-views:v1";
const MIGRATED_KEY = "kandev:jira:saved-views:migrated-to-backend:v1";
const SYNC_FAILED_KEY = "kandev:jira:saved-views:sync-failed:v1";

const BUILTIN_VIEWS: SavedView[] = [
  {
    id: "builtin:assigned",
    name: "Assigned to me",
    builtin: true,
    filters: { ...DEFAULT_FILTERS, assignee: "me" },
  },
  {
    id: "builtin:in-progress",
    name: "In progress",
    builtin: true,
    filters: { ...DEFAULT_FILTERS, assignee: "me", statusCategories: ["indeterminate"] },
  },
  {
    id: "builtin:unassigned",
    name: "Unassigned",
    builtin: true,
    filters: { ...DEFAULT_FILTERS, assignee: "unassigned" },
  },
];

function readStorage(): SavedView[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isSavedView);
  } catch {
    return [];
  }
}

function isFilterState(f: unknown): f is FilterState {
  if (!f || typeof f !== "object") return false;
  const rec = f as Record<string, unknown>;
  return (
    Array.isArray(rec.projectKeys) &&
    rec.projectKeys.every((k) => typeof k === "string") &&
    Array.isArray(rec.statusCategories) &&
    rec.statusCategories.every(
      (c) => c === "" || c === "new" || c === "indeterminate" || c === "done",
    ) &&
    (rec.assignee === "me" || rec.assignee === "unassigned" || rec.assignee === "anyone") &&
    typeof rec.searchText === "string" &&
    (rec.sort === "updated" || rec.sort === "created" || rec.sort === "priority")
  );
}

function isSavedView(v: unknown): v is SavedView {
  if (!v || typeof v !== "object") return false;
  const rec = v as Record<string, unknown>;
  return typeof rec.id === "string" && typeof rec.name === "string" && isFilterState(rec.filters);
}

function writeStorage(views: SavedView[]): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(views));
  } catch {
    // Quota or private-mode: swallow. Views just won't persist.
  }
}

function readServerViews(value: unknown): SavedView[] | null {
  if (!Array.isArray(value)) return null;
  return value.filter(isSavedView);
}

function syncServer(views: SavedView[]): Promise<void> {
  return updateUserSettings({ jira_saved_views: views })
    .then(() => {
      setUserSettingsSyncFailure(SYNC_FAILED_KEY, false);
    })
    .catch(() => {
      setUserSettingsSyncFailure(SYNC_FAILED_KEY, true);
    });
}

function snapshotKey(views: SavedView[]): string {
  return JSON.stringify(views);
}

function hasMigratedToBackend(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(MIGRATED_KEY) === "1";
}

function markMigratedToBackend(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(MIGRATED_KEY, "1");
  } catch {
    // Ignore write failures.
  }
}

function latestSavedViews(fallback: SavedView[]): SavedView[] {
  const stored = readStorage();
  return stored.length > 0 ? stored : fallback;
}

export function useSavedViews() {
  const [custom, setCustom] = useState<SavedView[]>([]);

  useEffect(() => {
    let cancelled = false;
    async function init() {
      const loaded = readStorage();
      const initialKey = snapshotKey(loaded);
      if (!cancelled) setCustom(loaded);
      const response = await fetchUserSettings({ cache: "no-store" }).catch(() => null);
      const serverViews = readServerViews(response?.settings.jira_saved_views);
      if (!cancelled && serverViews) {
        const local = readStorage();
        if (snapshotKey(local) !== initialKey) return;
        if (hasUserSettingsSyncFailure(SYNC_FAILED_KEY) && local.length > 0) {
          void syncServer(local);
          return;
        }
        if (serverViews.length === 0 && local.length > 0 && !hasMigratedToBackend()) {
          void syncServer(local);
          markMigratedToBackend();
          return;
        }
        writeStorage(serverViews);
        setCustom(serverViews);
        markMigratedToBackend();
      }
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, []);

  const save = useCallback(
    (name: string, filters: FilterState, customJql: string | null): SavedView => {
      const view: SavedView = {
        id: `custom:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
        name,
        filters,
        customJql,
      };
      setCustom((prev) => {
        const next = [...latestSavedViews(prev), view];
        writeStorage(next);
        void syncServer(next);
        markMigratedToBackend();
        return next;
      });
      return view;
    },
    [],
  );

  const remove = useCallback((id: string) => {
    setCustom((prev) => {
      const next = latestSavedViews(prev).filter((v) => v.id !== id);
      writeStorage(next);
      void syncServer(next);
      markMigratedToBackend();
      return next;
    });
  }, []);

  return {
    views: [...BUILTIN_VIEWS, ...custom],
    builtin: BUILTIN_VIEWS,
    custom,
    save,
    remove,
  };
}

export const DEFAULT_VIEW = BUILTIN_VIEWS[0];
