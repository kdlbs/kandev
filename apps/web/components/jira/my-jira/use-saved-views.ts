"use client";

import { useCallback, useEffect, useState } from "react";
import { DEFAULT_FILTERS, type FilterState } from "./filter-model";

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

export function useSavedViews() {
  const [custom, setCustom] = useState<SavedView[]>([]);

  useEffect(() => {
    let cancelled = false;
    async function init() {
      const loaded = readStorage();
      if (!cancelled) setCustom(loaded);
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
        const next = [...prev, view];
        writeStorage(next);
        return next;
      });
      return view;
    },
    [],
  );

  const remove = useCallback((id: string) => {
    setCustom((prev) => {
      const next = prev.filter((v) => v.id !== id);
      writeStorage(next);
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
