"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DEFAULT_FILTERS, type FilterState } from "./filter-model";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";

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

const BUILTIN_VIEWS: SavedView[] = [
  {
    id: "builtin:assigned",
    name: "Assigned to me",
    builtin: true,
    filters: { ...DEFAULT_FILTERS, assignee: "me" },
  },
  // The former "In progress" builtin filtered by the "indeterminate" status
  // category. Status names are now project-specific, so a global builtin can't
  // hard-code an in-progress status without a selected project; dropping the
  // status filter would have left it indistinguishable from "Assigned to me".
  // Users get an in-progress view by selecting a project and its statuses.
  {
    id: "builtin:unassigned",
    name: "Unassigned",
    builtin: true,
    filters: { ...DEFAULT_FILTERS, assignee: "unassigned" },
  },
];

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

function readServerViews(value: unknown): SavedView[] | null {
  if (!Array.isArray(value)) return null;
  return normalizeSavedViews(value);
}

const syncServer = createQueuedUserSettingsSync<SavedView[]>((views) => ({
  jira_saved_views: views,
}));

type PendingMutation = (views: SavedView[]) => SavedView[];

export function useSavedViews() {
  const [custom, setCustom] = useState<SavedView[]>([]);
  const customRef = useRef(custom);
  const hydrated = useRef(false);
  const pendingMutations = useRef<PendingMutation[]>([]);

  useEffect(() => {
    let cancelled = false;
    async function init() {
      const response = await fetchUserSettings({ cache: "no-store" }).catch(() => null);
      const serverViews = readServerViews(response?.settings.jira_saved_views);
      if (cancelled) return;

      const mutations = pendingMutations.current;
      pendingMutations.current = [];
      const hydratedViews = mutations.reduce((views, mutate) => mutate(views), serverViews ?? []);
      hydrated.current = true;
      customRef.current = hydratedViews;
      setCustom(hydratedViews);
      if (mutations.length > 0) {
        void syncServer(hydratedViews);
      }
    }
    void init();
    return () => {
      cancelled = true;
    };
  }, []);

  const commitMutation = useCallback((mutate: PendingMutation) => {
    const next = mutate(customRef.current);
    customRef.current = next;
    setCustom(next);
    void syncServer(next);
  }, []);

  const save = useCallback(
    (name: string, filters: FilterState, customJql: string | null): SavedView => {
      const view: SavedView = {
        id: `custom:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
        name,
        filters,
        customJql,
      };
      const mutate: PendingMutation = (views) => [...views, view];
      if (!hydrated.current) {
        pendingMutations.current.push(mutate);
        return view;
      }
      commitMutation(mutate);
      return view;
    },
    [commitMutation],
  );

  const remove = useCallback(
    (id: string) => {
      const mutate: PendingMutation = (views) => views.filter((v) => v.id !== id);
      if (!hydrated.current) {
        pendingMutations.current.push(mutate);
        return;
      }
      commitMutation(mutate);
    },
    [commitMutation],
  );

  return {
    views: [...BUILTIN_VIEWS, ...custom],
    builtin: BUILTIN_VIEWS,
    custom,
    save,
    remove,
  };
}

export const DEFAULT_VIEW = BUILTIN_VIEWS[0];
