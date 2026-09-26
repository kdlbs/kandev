"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { DEFAULT_FILTERS, type FilterState } from "./filter-model";
import { fetchUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

export type SavedView = {
  id: string;
  /** User-authored for custom views; empty for builtins, which use `nameKey`. */
  name: string;
  /** Catalog key for the built-in views, resolved where the view is listed. */
  nameKey?: string;
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
    name: "",
    nameKey: "jira:builtinViewAssignedToMe",
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
    name: "",
    nameKey: "jira:builtinViewUnassigned",
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

type SettingsSync = (patch: UserSettingsUpdatePayload) => Promise<unknown>;

type PendingMutation = (views: SavedView[]) => SavedView[];

type SavedViewsHydrationState = {
  hydrationRequest: { current: Promise<void> | null };
  hydrated: { current: boolean };
  mounted: { current: boolean };
  hydrationError: { current: unknown };
  customRef: { current: SavedView[] };
  defaultViewIdRef: { current: string };
  setCustom: Dispatch<SetStateAction<SavedView[]>>;
  setDefaultViewId: Dispatch<SetStateAction<string>>;
  setReady: Dispatch<SetStateAction<boolean>>;
};

async function hydrateSavedViewSettings(state: SavedViewsHydrationState): Promise<void> {
  const activeRequest = state.hydrationRequest.current;
  if (activeRequest) {
    await activeRequest;
    if (state.hydrated.current || !state.mounted.current) return;
  }

  const request = (async () => {
    const response = await fetchUserSettings({ cache: "no-store" }).catch((error: unknown) => {
      state.hydrationError.current = error;
      return null;
    });
    if (!state.mounted.current) return;
    if (!response) {
      state.setReady(true);
      return;
    }
    const serverViews = readServerViews(response.settings.jira_saved_views);
    const serverDefaultViewId =
      typeof response.settings.jira_default_view_id === "string"
        ? response.settings.jira_default_view_id.trim()
        : "";

    state.hydrationError.current = null;
    state.hydrated.current = true;
    state.customRef.current = serverViews ?? [];
    state.defaultViewIdRef.current = serverDefaultViewId;
    state.setCustom(serverViews ?? []);
    state.setDefaultViewId(serverDefaultViewId);
    state.setReady(true);
  })();
  state.hydrationRequest.current = request;
  try {
    await request;
  } finally {
    state.hydrationRequest.current = null;
  }
}

function useSavedViewsState() {
  const settingsSyncRef = useRef<SettingsSync | null>(null);
  if (!settingsSyncRef.current) {
    settingsSyncRef.current = createQueuedUserSettingsSyncWithResponse<UserSettingsUpdatePayload>(
      (patch) => patch,
    );
  }
  const syncSettings = settingsSyncRef.current;
  const [custom, setCustom] = useState<SavedView[]>([]);
  const [defaultViewId, setDefaultViewId] = useState("");
  const [ready, setReady] = useState(false);
  const [defaultMutationPending, setDefaultMutationPending] = useState(false);
  const [viewMutationPending, setViewMutationPending] = useState(false);
  const customRef = useRef(custom);
  const defaultViewIdRef = useRef(defaultViewId);
  const hydrated = useRef(false);
  const mounted = useRef(true);
  const hydrationError = useRef<unknown>(null);
  const defaultMutationPendingRef = useRef(false);
  const viewMutationPendingRef = useRef(false);
  const hydrationRequest = useRef<Promise<void> | null>(null);

  const hydrate = useCallback(
    () =>
      hydrateSavedViewSettings({
        hydrationRequest,
        hydrated,
        mounted,
        hydrationError,
        customRef,
        defaultViewIdRef,
        setCustom,
        setDefaultViewId,
        setReady,
      }),
    [],
  );

  useEffect(() => {
    mounted.current = true;
    void hydrate();
    return () => {
      mounted.current = false;
    };
  }, [hydrate]);

  const ensureHydrated = useCallback(async () => {
    if (!hydrated.current) await hydrate();
    if (!hydrated.current) throw hydrationError.current ?? new Error();
  }, [hydrate]);

  return {
    syncSettings,
    custom,
    setCustom,
    defaultViewId,
    setDefaultViewId,
    ready,
    setReady,
    defaultMutationPending,
    setDefaultMutationPending,
    viewMutationPending,
    setViewMutationPending,
    customRef,
    defaultViewIdRef,
    hydrated,
    mounted,
    hydrationError,
    defaultMutationPendingRef,
    viewMutationPendingRef,
    hydrate,
    ensureHydrated,
  };
}

type SavedViewsState = ReturnType<typeof useSavedViewsState>;

function useCommitSavedViewMutation(state: SavedViewsState) {
  return useCallback(
    async (mutate: PendingMutation) => {
      if (state.viewMutationPendingRef.current || state.defaultMutationPendingRef.current) {
        throw new Error();
      }
      state.viewMutationPendingRef.current = true;
      state.setViewMutationPending(true);
      try {
        const next = mutate(state.customRef.current);
        await state.syncSettings({ jira_saved_views: next });
        if (!state.mounted.current) return false;
        state.customRef.current = next;
        state.setCustom(next);
        return true;
      } finally {
        state.viewMutationPendingRef.current = false;
        if (state.mounted.current) state.setViewMutationPending(false);
      }
    },
    [state],
  );
}

async function removeDefaultView(id: string, state: SavedViewsState): Promise<boolean> {
  if (state.viewMutationPendingRef.current || state.defaultMutationPendingRef.current) return false;
  state.viewMutationPendingRef.current = true;
  state.setViewMutationPending(true);
  state.defaultMutationPendingRef.current = true;
  state.setDefaultMutationPending(true);
  try {
    const next = state.customRef.current.filter((view) => view.id !== id);
    await state.syncSettings({ jira_saved_views: next, jira_default_view_id: "" });
    if (!state.mounted.current) return false;
    state.customRef.current = next;
    state.setCustom(next);
    state.defaultViewIdRef.current = "";
    state.setDefaultViewId("");
    return true;
  } finally {
    state.viewMutationPendingRef.current = false;
    if (state.mounted.current) state.setViewMutationPending(false);
    state.defaultMutationPendingRef.current = false;
    if (state.mounted.current) state.setDefaultMutationPending(false);
  }
}

function useSavedViewMutations(state: SavedViewsState) {
  const commitMutation = useCommitSavedViewMutation(state);
  const { defaultMutationPendingRef, ensureHydrated, viewMutationPendingRef } = state;

  const save = useCallback(
    async (name: string, filters: FilterState, customJql: string | null): Promise<SavedView> => {
      if (viewMutationPendingRef.current || defaultMutationPendingRef.current) throw new Error();
      await ensureHydrated();
      const view: SavedView = {
        id: `custom:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
        name,
        filters,
        customJql,
      };
      const mutate: PendingMutation = (views) => [...views, view];
      const committed = await commitMutation(mutate);
      if (!committed) throw new Error();
      return view;
    },
    [commitMutation, defaultMutationPendingRef, ensureHydrated, viewMutationPendingRef],
  );

  const remove = useCallback(
    async (id: string) => {
      if (state.viewMutationPendingRef.current || state.defaultMutationPendingRef.current) {
        return false;
      }
      await state.ensureHydrated();
      if (!state.customRef.current.some((view) => view.id === id)) return false;
      if (state.defaultViewIdRef.current === id) return removeDefaultView(id, state);
      const mutate: PendingMutation = (views) => views.filter((view) => view.id !== id);
      return commitMutation(mutate);
    },
    [commitMutation, state],
  );

  const setDefaultView = useCallback(
    async (id: string) => {
      if (state.defaultMutationPendingRef.current || state.viewMutationPendingRef.current)
        return false;
      const next = id.trim();
      if (
        next &&
        !BUILTIN_VIEWS.some((view) => view.id === next) &&
        !state.customRef.current.some((view) => view.id === next)
      )
        return false;
      if (state.defaultViewIdRef.current === next) return true;
      state.defaultMutationPendingRef.current = true;
      state.setDefaultMutationPending(true);
      try {
        await state.syncSettings({ jira_default_view_id: next });
        if (!state.mounted.current) return false;
        state.defaultViewIdRef.current = next;
        state.setDefaultViewId(next);
        return true;
      } finally {
        state.defaultMutationPendingRef.current = false;
        if (state.mounted.current) state.setDefaultMutationPending(false);
      }
    },
    [state],
  );

  return { save, remove, setDefaultView };
}

export function useSavedViews() {
  const state = useSavedViewsState();
  const mutations = useSavedViewMutations(state);
  return {
    views: [...BUILTIN_VIEWS, ...state.custom],
    builtin: BUILTIN_VIEWS,
    custom: state.custom,
    defaultViewId: state.defaultViewId,
    ready: state.ready,
    defaultMutationPending: state.defaultMutationPending,
    viewMutationPending: state.viewMutationPending,
    ...mutations,
  };
}

export const DEFAULT_VIEW = BUILTIN_VIEWS[0];

/**
 * Display name for a view: the catalog copy for a builtin, the user's own
 * wording for a saved one. Takes `t` so it resolves at render.
 */
export function savedViewLabel(translate: (key: string) => string, view: SavedView): string {
  return view.nameKey ? translate(view.nameKey) : view.name;
}
