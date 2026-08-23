import { updateUserSettingsWithRetry } from "@/lib/user-settings-sync";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import type { UISlice, UISliceState } from "./types";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "./sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";
import { toApiSidebarDraft, toApiSidebarView } from "./sidebar-view-wire";
import { createDefaultSidebarView, MAX_SIDEBAR_VIEWS } from "./sidebar-view-builtins";
import { t } from "@/lib/i18n";

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

function nextNewViewName(views: SidebarView[]): string {
  const names = new Set(views.map((view) => view.name));
  const base = t("sidebar:newView");
  if (!names.has(base)) return base;
  let suffix = 2;
  while (names.has(t("sidebar:newViewNumbered", { suffix }))) suffix += 1;
  return t("sidebar:newViewNumbered", { suffix });
}

const sidebarSettingsQueues = new WeakMap<ImmerSet, Promise<void>>();

type SidebarSnapshot = {
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
};

type SidebarWriteJournal = {
  latestRequestId: number;
  failedRollback?: SidebarSnapshot;
  failedWriteKind?: SidebarWriteKind;
};

type SidebarWriteKind = "views" | "local";

const sidebarWriteJournals = new WeakMap<ImmerSet, SidebarWriteJournal>();

function getSidebarWriteJournal(set: ImmerSet): SidebarWriteJournal {
  const existing = sidebarWriteJournals.get(set);
  if (existing) return existing;
  const created = { latestRequestId: 0 };
  sidebarWriteJournals.set(set, created);
  return created;
}

function snapshotSidebar(s: UISliceState["sidebarViews"]): SidebarSnapshot {
  return {
    views: s.views.map(cloneView),
    activeViewId: s.activeViewId,
    draft: cloneDraft(s.draft),
  };
}

function toSidebarSettingsPayload(s: SidebarSnapshot | UISliceState["sidebarViews"]) {
  return {
    sidebar_views: s.views.map(toApiSidebarView),
    sidebar_active_view_id: s.activeViewId,
    sidebar_draft: s.draft ? toApiSidebarDraft(s.draft) : null,
  };
}

function rollbackSidebarState(
  set: ImmerSet,
  rollback: SidebarSnapshot,
  after: SidebarSnapshot,
  error?: unknown,
): void {
  set((draft) => {
    draft.sidebarViews.views = rollback.views;
    const activeViewStillExists = rollback.views.some(
      (view) => view.id === draft.sidebarViews.activeViewId,
    );
    if (draft.sidebarViews.activeViewId === after.activeViewId || !activeViewStillExists) {
      draft.sidebarViews.activeViewId = rollback.activeViewId;
    }
    const currentDraft = draft.sidebarViews.draft;
    const draftBaseStillExists =
      !currentDraft || rollback.views.some((view) => view.id === currentDraft.baseViewId);
    if (draftsEqual(currentDraft, after.draft) || !draftBaseStillExists) {
      draft.sidebarViews.draft = rollback.draft;
    }
    if (error !== undefined) {
      draft.sidebarViews.syncError =
        error instanceof Error ? error.message : t("sidebar:failedToSyncSidebarViews");
    }
  });
}

function restoreFailedViewMutation(set: ImmerSet, rollback: SidebarSnapshot): void {
  set((draft) => {
    draft.sidebarViews.views = rollback.views;
    if (!rollback.views.some((view) => view.id === draft.sidebarViews.activeViewId)) {
      draft.sidebarViews.activeViewId = rollback.activeViewId;
    }
    const currentDraft = draft.sidebarViews.draft;
    if (currentDraft && !rollback.views.some((view) => view.id === currentDraft.baseViewId)) {
      draft.sidebarViews.draft = rollback.draft;
    }
  });
}

function draftsEqual(a: SidebarViewDraft | null, b: SidebarViewDraft | null): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

function enqueueSidebarSettingsSync(
  set: ImmerSet,
  payload: UserSettingsUpdatePayload,
): Promise<void> {
  const previous = sidebarSettingsQueues.get(set);
  const request = previous
    ? previous.then(() => updateUserSettingsWithRetry(payload))
    : updateUserSettingsWithRetry(payload);
  sidebarSettingsQueues.set(
    set,
    request.catch(() => undefined),
  );
  return request;
}

function syncSidebarWrite(
  set: ImmerSet,
  before: SidebarSnapshot,
  after: SidebarSnapshot,
  payload: UserSettingsUpdatePayload,
  kind: SidebarWriteKind,
) {
  const journal = getSidebarWriteJournal(set);
  const thisRequestId = ++journal.latestRequestId;
  enqueueSidebarSettingsSync(set, payload).then(
    () => {
      if (
        thisRequestId === journal.latestRequestId &&
        journal.failedWriteKind === "views" &&
        kind === "local" &&
        journal.failedRollback
      ) {
        restoreFailedViewMutation(set, journal.failedRollback);
      }
      journal.failedRollback = undefined;
      journal.failedWriteKind = undefined;
    },
    (err) => {
      const rollback = journal.failedRollback ?? before;
      journal.failedRollback = rollback;
      journal.failedWriteKind ??= kind;
      set((draft) => {
        draft.sidebarViews.syncError =
          err instanceof Error ? err.message : t("sidebar:failedToSyncSidebarViews");
      });
      if (thisRequestId !== journal.latestRequestId) return;
      rollbackSidebarState(set, rollback, after, err);
      journal.failedRollback = undefined;
      journal.failedWriteKind = undefined;
    },
  );
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
  syncSidebarWrite(set, snapshot, afterSnapshot, toSidebarSettingsPayload(after), "views");
}

function buildSidebarLocalActions(set: ImmerSet, get: () => UISlice) {
  return {
    setSidebarActiveView: (viewId: string) => {
      const before = snapshotSidebar(get().sidebarViews);
      let committed = false;
      set((draft) => {
        if (!draft.sidebarViews.views.some((v) => v.id === viewId)) return;
        committed = true;
        draft.sidebarViews.activeViewId = viewId;
        draft.sidebarViews.draft = null;
      });
      if (!committed) return;
      const after = snapshotSidebar(get().sidebarViews);
      syncSidebarWrite(
        set,
        before,
        after,
        {
          sidebar_active_view_id: after.activeViewId,
          sidebar_draft: after.draft ? toApiSidebarDraft(after.draft) : null,
        },
        "local",
      );
    },
    updateSidebarDraft: (
      patch: Partial<{
        filters: FilterClause[];
        sort: SortSpec;
        group: GroupKey;
        taskRow: SidebarView["taskRow"];
      }>,
    ) => {
      const before = snapshotSidebar(get().sidebarViews);
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
          taskRow: cloneSidebarTaskRowPresentation(active.taskRow),
        };
        const next: SidebarViewDraft = {
          baseViewId: active.id,
          filters: patch.filters ?? current.filters,
          sort: patch.sort ?? current.sort,
          group: patch.group ?? current.group,
          taskRow: cloneSidebarTaskRowPresentation(patch.taskRow ?? current.taskRow),
        };
        draft.sidebarViews.draft = next;
      });
      if (!committed) return;
      const after = snapshotSidebar(get().sidebarViews);
      syncSidebarWrite(
        set,
        before,
        after,
        {
          sidebar_active_view_id: after.activeViewId,
          sidebar_draft: after.draft ? toApiSidebarDraft(after.draft) : null,
        },
        "local",
      );
    },
    discardSidebarDraft: () => {
      const before = snapshotSidebar(get().sidebarViews);
      set((draft) => {
        draft.sidebarViews.draft = null;
      });
      const after = snapshotSidebar(get().sidebarViews);
      syncSidebarWrite(
        set,
        before,
        after,
        {
          sidebar_active_view_id: after.activeViewId,
          sidebar_draft: null,
        },
        "local",
      );
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
    createSidebarView: () => {
      let createdViewId: string | null = null;
      mv((s) => {
        if (s.draft || s.views.length >= MAX_SIDEBAR_VIEWS) return false;
        const view = createDefaultSidebarView(makeId("view"), nextNewViewName(s.views));
        s.views.push(view);
        s.activeViewId = view.id;
        createdViewId = view.id;
      });
      return createdViewId;
    },
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
          name: name.trim() || t("sidebar:untitledView"),
          filters: s.draft.filters,
          sort: s.draft.sort,
          group: s.draft.group,
          collapsedGroups: [],
          taskRow: cloneSidebarTaskRowPresentation(s.draft.taskRow),
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
        view.taskRow = cloneSidebarTaskRowPresentation(s.draft.taskRow);
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
          taskRow: source.taskRow ? cloneSidebarTaskRowPresentation(source.taskRow) : undefined,
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
    taskRow: v.taskRow ? cloneSidebarTaskRowPresentation(v.taskRow) : undefined,
  };
}

function cloneDraft(draft: SidebarViewDraft | null): SidebarViewDraft | null {
  if (!draft) return null;
  return {
    baseViewId: draft.baseViewId,
    filters: draft.filters.map((filter) => ({ ...filter })),
    sort: { ...draft.sort },
    group: draft.group,
    taskRow: draft.taskRow ? cloneSidebarTaskRowPresentation(draft.taskRow) : undefined,
  };
}
