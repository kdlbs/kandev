import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UISlice, UISliceState } from "./types";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "./sidebar-view-types";
import { toApiSidebarDraft, toApiSidebarView } from "./sidebar-view-wire";

type ImmerSet = (recipe: (draft: UISlice) => void, shouldReplace?: false | undefined) => void;

function makeId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function reorderViewsById(
  views: SidebarView[],
  activeViewId: string,
  overViewId: string,
): SidebarView[] | null {
  if (activeViewId === overViewId) return null;
  const oldIndex = views.findIndex((v) => v.id === activeViewId);
  const newIndex = views.findIndex((v) => v.id === overViewId);
  if (oldIndex === -1 || newIndex === -1) return null;
  const next = [...views];
  const [moved] = next.splice(oldIndex, 1);
  next.splice(newIndex, 0, moved);
  return next;
}

let viewsSyncRequestId = 0;

type SidebarSnapshot = {
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
};

function snapshotSidebar(s: UISliceState["sidebarViews"]): SidebarSnapshot {
  return {
    views: s.views.map(cloneView),
    activeViewId: s.activeViewId,
    draft: s.draft ? { ...s.draft } : null,
  };
}

function toSidebarSettingsPayload(s: SidebarSnapshot | UISliceState["sidebarViews"]) {
  return {
    sidebar_views: s.views.map(toApiSidebarView),
    sidebar_active_view_id: s.activeViewId,
    sidebar_draft: s.draft ? toApiSidebarDraft(s.draft) : null,
  };
}

function draftsEqual(a: SidebarViewDraft | null, b: SidebarViewDraft | null): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

function syncSidebarViewState(
  set: ImmerSet,
  payload: {
    sidebar_active_view_id: string;
    sidebar_draft: ReturnType<typeof toApiSidebarDraft> | null;
  },
) {
  updateUserSettings(payload).catch((err) => {
    const message = err instanceof Error ? err.message : "Failed to sync sidebar views";
    set((draft) => {
      draft.sidebarViews.syncError = message;
    });
  });
}

function mutateViews(
  set: ImmerSet,
  get: () => UISlice,
  mutate: (slice: UISliceState["sidebarViews"]) => boolean | void,
): void {
  const snapshot = snapshotSidebar(get().sidebarViews);
  let committed = false;
  set((draft) => {
    committed = mutate(draft.sidebarViews) !== false;
  });
  if (!committed) return;
  const after = get().sidebarViews;
  const afterSnapshot = snapshotSidebar(after);
  const thisRequestId = ++viewsSyncRequestId;
  updateUserSettings(toSidebarSettingsPayload(after)).catch((err) => {
    if (thisRequestId !== viewsSyncRequestId) return;
    const message = err instanceof Error ? err.message : "Failed to sync sidebar views";
    set((draft) => {
      draft.sidebarViews.views = snapshot.views;
      if (draft.sidebarViews.activeViewId === afterSnapshot.activeViewId) {
        draft.sidebarViews.activeViewId = snapshot.activeViewId;
      }
      if (draftsEqual(draft.sidebarViews.draft, afterSnapshot.draft)) {
        draft.sidebarViews.draft = snapshot.draft;
      }
      draft.sidebarViews.syncError = message;
    });
  });
}

function buildSidebarLocalActions(set: ImmerSet, get: () => UISlice) {
  return {
    setSidebarActiveView: (viewId: string) => {
      let committed = false;
      set((draft) => {
        if (!draft.sidebarViews.views.some((v) => v.id === viewId)) return;
        committed = true;
        draft.sidebarViews.activeViewId = viewId;
        draft.sidebarViews.draft = null;
      });
      if (!committed) return;
      syncSidebarViewState(set, { sidebar_active_view_id: viewId, sidebar_draft: null });
    },
    updateSidebarDraft: (
      patch: Partial<{ filters: FilterClause[]; sort: SortSpec; group: GroupKey }>,
    ) => {
      let committed = false;
      set((draft) => {
        const active = draft.sidebarViews.views.find(
          (v) => v.id === draft.sidebarViews.activeViewId,
        );
        if (!active) return;
        committed = true;
        const current: SidebarViewDraft = draft.sidebarViews.draft ?? {
          baseViewId: active.id,
          filters: active.filters,
          sort: active.sort,
          group: active.group,
        };
        const next: SidebarViewDraft = {
          baseViewId: active.id,
          filters: patch.filters ?? current.filters,
          sort: patch.sort ?? current.sort,
          group: patch.group ?? current.group,
        };
        draft.sidebarViews.draft = next;
      });
      if (!committed) return;
      const { activeViewId, draft } = get().sidebarViews;
      syncSidebarViewState(set, {
        sidebar_active_view_id: activeViewId,
        sidebar_draft: draft ? toApiSidebarDraft(draft) : null,
      });
    },
    discardSidebarDraft: () => {
      set((draft) => {
        draft.sidebarViews.draft = null;
      });
      syncSidebarViewState(set, {
        sidebar_active_view_id: get().sidebarViews.activeViewId,
        sidebar_draft: null,
      });
    },
    clearSidebarSyncError: () =>
      set((draft) => {
        draft.sidebarViews.syncError = null;
      }),
  };
}

function buildSidebarBackendActions(set: ImmerSet, get: () => UISlice) {
  const mv = (mutate: (s: UISliceState["sidebarViews"]) => boolean | void) =>
    mutateViews(set, get, mutate);
  return {
    toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) =>
      mv((s) => {
        const view = s.views.find((v) => v.id === viewId);
        if (!view) return false;
        const idx = view.collapsedGroups.indexOf(groupKey);
        if (idx === -1) view.collapsedGroups.push(groupKey);
        else view.collapsedGroups.splice(idx, 1);
      }),
    saveSidebarDraftAs: (name: string) =>
      mv((s) => {
        if (!s.draft) return false;
        s.views.push({
          id: makeId("view"),
          name: name.trim() || "Untitled view",
          filters: s.draft.filters,
          sort: s.draft.sort,
          group: s.draft.group,
          collapsedGroups: [],
        });
        s.activeViewId = s.views[s.views.length - 1].id;
        s.draft = null;
      }),
    saveSidebarDraftOverwrite: () =>
      mv((s) => {
        if (!s.draft) return false;
        const view = s.views.find((v) => v.id === s.draft!.baseViewId);
        if (!view) return false;
        view.filters = s.draft.filters;
        view.sort = s.draft.sort;
        view.group = s.draft.group;
        s.draft = null;
      }),
    duplicateSidebarView: (viewId: string, name: string) =>
      mv((s) => {
        const source = s.views.find((v) => v.id === viewId);
        if (!source) return false;
        s.views.push({
          id: makeId("view"),
          name: name.trim() || `${source.name} copy`,
          filters: source.filters.map((f) => ({ ...f, id: makeId("clause") })),
          sort: source.sort,
          group: source.group,
          collapsedGroups: [],
        });
        s.activeViewId = s.views[s.views.length - 1].id;
      }),
    deleteSidebarView: (viewId: string) =>
      mv((s) => {
        const remaining = s.views.filter((v) => v.id !== viewId);
        if (remaining.length === 0) return false;
        s.views = remaining;
        if (s.activeViewId === viewId) s.activeViewId = remaining[0].id;
        s.draft = null;
      }),
    renameSidebarView: (viewId: string, name: string) =>
      mv((s) => {
        const view = s.views.find((v) => v.id === viewId);
        if (!view) return false;
        const next = name.trim();
        if (!next || next === view.name) return false;
        view.name = next;
      }),
    reorderSidebarViews: (activeViewId: string, overViewId: string) =>
      mv((s) => {
        const reordered = reorderViewsById(s.views, activeViewId, overViewId);
        if (!reordered) return false;
        s.views = reordered;
      }),
  };
}

export function buildSidebarViewActions(set: ImmerSet, get: () => UISlice) {
  return {
    ...buildSidebarLocalActions(set, get),
    ...buildSidebarBackendActions(set, get),
  };
}

function cloneView(v: SidebarView): SidebarView {
  return {
    id: v.id,
    name: v.name,
    filters: v.filters.map((f) => ({ ...f })),
    sort: { ...v.sort },
    group: v.group,
    collapsedGroups: [...v.collapsedGroups],
  };
}
