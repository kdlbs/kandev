"use client";

import { useCallback, useEffect, useState } from "react";
import { DEFAULT_FILTERS, type FilterState } from "./filter-model";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";
import { hasUserSettingsSyncFailure } from "@/lib/user-settings-sync-failure";

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
    // Status names are project-specific, so a global builtin can't hard-code an
    // "In Progress" status. Scope to the current user; the user layers on the
    // project-specific status filter once a project is selected.
    filters: { ...DEFAULT_FILTERS, assignee: "me" },
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
    return normalizeSavedViews(parsed);
  } catch {
    return [];
  }
}

// isFilterStateShape recognizes both the current `statuses: string[]` shape and
// legacy views persisted with `statusCategories`. It only validates the fields
// unrelated to statuses; status normalization happens in normalizeFilterState.
function isFilterStateShape(f: unknown): f is Record<string, unknown> {
  if (!f || typeof f !== "object") return false;
  const rec = f as Record<string, unknown>;
  return (
    Array.isArray(rec.projectKeys) &&
    rec.projectKeys.every((k) => typeof k === "string") &&
    (rec.assignee === "me" || rec.assignee === "unassigned" || rec.assignee === "anyone") &&
    typeof rec.searchText === "string" &&
    (rec.sort === "updated" || rec.sort === "created" || rec.sort === "priority")
  );
}

// normalizeFilterState coerces a persisted filter (current or legacy) into a
// valid FilterState. Legacy views carrying `statusCategories` (and no
// `statuses`) hydrate to `statuses: []` — the old category filter is dropped
// rather than throwing, since categories no longer map to a specific status.
function normalizeFilterState(rec: Record<string, unknown>): FilterState {
  const statuses =
    Array.isArray(rec.statuses) && rec.statuses.every((s) => typeof s === "string")
      ? (rec.statuses as string[])
      : [];
  return {
    projectKeys: rec.projectKeys as string[],
    statuses,
    assignee: rec.assignee as FilterState["assignee"],
    searchText: rec.searchText as string,
    sort: rec.sort as FilterState["sort"],
  };
}

function normalizeSavedView(v: unknown): SavedView | null {
  if (!v || typeof v !== "object") return null;
  const rec = v as Record<string, unknown>;
  if (typeof rec.id !== "string" || typeof rec.name !== "string") return null;
  if (!isFilterStateShape(rec.filters)) return null;
  return {
    id: rec.id,
    name: rec.name,
    filters: normalizeFilterState(rec.filters),
    customJql: typeof rec.customJql === "string" ? rec.customJql : null,
    builtin: rec.builtin === true,
  };
}

function normalizeSavedViews(values: unknown[]): SavedView[] {
  return values.map(normalizeSavedView).filter((v): v is SavedView => v !== null);
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
  return normalizeSavedViews(value);
}

const syncServer = createQueuedUserSettingsSync<SavedView[]>(SYNC_FAILED_KEY, (views) => ({
  jira_saved_views: views,
}));

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
  if (typeof window === "undefined") return fallback;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw === null) return fallback;
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return fallback;
    return normalizeSavedViews(parsed);
  } catch {
    return fallback;
  }
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
        if (hasUserSettingsSyncFailure(SYNC_FAILED_KEY)) {
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
