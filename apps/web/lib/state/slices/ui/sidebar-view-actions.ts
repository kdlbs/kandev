import { createSidebarWorkspaceState, mapSidebarWorkspaces } from "./sidebar-workspace-state";
import type { WorkspaceSliceState } from "../workspace/types";
import { requestUserSettingsUpdateWithRetry } from "@/lib/user-settings-sync";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import type { UISlice, UISliceState } from "./types";
import type { FilterClause, GroupKey, SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";
import { toApiSidebarDraft } from "./sidebar-view-wire";
import { sidebarViewDelta } from "./sidebar-view-delta";
import { cloneView, cloneDraft } from "./sidebar-view-clone";
import { createDefaultSidebarView, MAX_SIDEBAR_VIEWS } from "./sidebar-view-builtins";
import { t } from "@/lib/i18n";
import {
  createSidebarPluginView,
  findGeneratedSidebarPluginView,
  isSidebarPluginGrouping,
  type SidebarPluginGroupingChoice,
} from "@/lib/sidebar/sidebar-plugin-grouping";

type SidebarViewPatch = Omit<
  NonNullable<UserSettingsUpdatePayload["sidebar_view_state"]>,
  "workspace_id"
>;
type SidebarActionState = Pick<UISliceState, "sidebarViews">;
type ImmerSet = (
  recipe: (draft: SidebarActionState) => void,
  shouldReplace?: false | undefined,
) => void;
const workspaceBySetter = new WeakMap<ImmerSet, string>();

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
  pending: number;
  confirmed?: SidebarSnapshot;
  failedRollback?: SidebarSnapshot;
  failedWriteKind?: SidebarWriteKind;
};

type SidebarWriteKind = "views" | "local";

const sidebarWriteJournals = new WeakMap<ImmerSet, SidebarWriteJournal>();

function getSidebarWriteJournal(set: ImmerSet): SidebarWriteJournal {
  const existing = sidebarWriteJournals.get(set);
  if (existing) return existing;
  const created: SidebarWriteJournal = { latestRequestId: 0, pending: 0 };
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

function toSidebarSettingsDelta(before: SidebarSnapshot, after: SidebarSnapshot) {
  return {
    ...sidebarViewDelta(before.views, after.views),
    active_view_id: after.activeViewId,
    draft: after.draft ? toApiSidebarDraft(after.draft) : null,
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
  payload: () => SidebarViewPatch,
  after: SidebarSnapshot,
  kind: SidebarWriteKind,
): Promise<void> {
  const workspaceId = workspaceBySetter.get(set);
  if (!workspaceId) {
    // i18n-exempt: programmer invariant diagnostic; this setter is registered before use.
    return Promise.reject(new Error("sidebar set not bound to a workspace"));
  }
  const send = () =>
    requestUserSettingsUpdateWithRetry({
      sidebar_view_state: { workspace_id: workspaceId, ...payload() },
    });
  const previous = sidebarSettingsQueues.get(set);
  const write = previous ? previous.then(send) : send();
  return write.then((response) => {
    const server = response.settings.sidebar_views_by_workspace?.[workspaceId];
    const journal = getSidebarWriteJournal(set);
    journal.confirmed = server
      ? snapshotSidebar(mapSidebarWorkspaces({ [workspaceId]: server })[workspaceId])
      : { ...after, views: kind === "views" ? after.views : journal.confirmed!.views };
    if (!server) return;
    set((state) => {
      const mapped = mapSidebarWorkspaces(
        { [workspaceId]: server },
        { [workspaceId]: state.sidebarViews },
        response.settings.revision,
      );
      Object.assign(state.sidebarViews, mapped[workspaceId]);
    });
  });
}

function syncSidebarWrite(
  set: ImmerSet,
  before: SidebarSnapshot,
  after: SidebarSnapshot,
  payload: SidebarViewPatch | ((confirmed: SidebarSnapshot) => SidebarViewPatch),
  kind: SidebarWriteKind,
) {
  const journal = getSidebarWriteJournal(set);
  if (journal.pending === 0) journal.confirmed = before;
  journal.pending++;
  const thisRequestId = ++journal.latestRequestId;
  set((state) => {
    state.sidebarViews.syncPending = true;
  });
  const completed = enqueueSidebarSettingsSync(
    set,
    () => (typeof payload === "function" ? payload(journal.confirmed!) : payload),
    after,
    kind,
  ).then(
    () => {
      journal.pending--;
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
      if (thisRequestId === journal.latestRequestId)
        set((state) => {
          const server = state.sidebarViews.deferredServerState;
          if (server) Object.assign(state.sidebarViews, server);
          state.sidebarViews.syncPending = false;
          state.sidebarViews.deferredServerState = null;
        });
    },
    (err) => {
      journal.pending--;
      const rollback = journal.failedRollback ?? before;
      journal.failedRollback = rollback;
      journal.failedWriteKind ??= kind;
      set((draft) => {
        draft.sidebarViews.syncError =
          err instanceof Error ? err.message : t("sidebar:failedToSyncSidebarViews");
      });
      if (thisRequestId !== journal.latestRequestId) return;
      rollbackSidebarState(set, rollback, after, err);
      set((state) => {
        const server = state.sidebarViews.deferredServerState;
        if (server) {
          state.sidebarViews.views = server.views;
          state.sidebarViews.activeViewId = server.activeViewId;
          state.sidebarViews.draft = server.draft;
        }
        state.sidebarViews.syncPending = false;
        state.sidebarViews.deferredServerState = null;
      });
      journal.failedRollback = undefined;
      journal.failedWriteKind = undefined;
    },
  );
  sidebarSettingsQueues.set(set, completed);
}

function mutateViews(
  set: ImmerSet,
  get: () => SidebarActionState,
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
  syncSidebarWrite(
    set,
    snapshot,
    afterSnapshot,
    (confirmed) => toSidebarSettingsDelta(confirmed, afterSnapshot),
    "views",
  );
}

type SidebarDraftPatch = Partial<{
  filters: FilterClause[];
  sort: SidebarView["sort"];
  group: GroupKey;
  groupIndent: boolean;
  taskRow: SidebarView["taskRow"];
}>;

function updateSidebarDraft(
  set: ImmerSet,
  get: () => SidebarActionState,
  patch: SidebarDraftPatch,
): void {
  const before = snapshotSidebar(get().sidebarViews);
  let committed = false;
  set((draft) => {
    const active = draft.sidebarViews.views.find((v) => v.id === draft.sidebarViews.activeViewId);
    if (!active) return;
    committed = true;
    const current: SidebarViewDraft = draft.sidebarViews.draft ?? {
      baseViewId: active.id,
      filters: active.filters,
      sort: active.sort,
      ...(active.sortWarningCount !== undefined
        ? { sortWarningCount: active.sortWarningCount }
        : {}),
      group: active.group,
      groupIndent: active.groupIndent,
      taskRow: cloneSidebarTaskRowPresentation(active.taskRow),
    };
    const next: SidebarViewDraft = {
      baseViewId: active.id,
      filters: patch.filters ?? current.filters,
      sort: patch.sort ?? current.sort,
      ...(current.sortWarningCount !== undefined
        ? { sortWarningCount: current.sortWarningCount }
        : {}),
      group: patch.group ?? current.group,
      groupIndent: patch.groupIndent ?? current.groupIndent,
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
      active_view_id: after.activeViewId,
      draft: after.draft ? toApiSidebarDraft(after.draft) : null,
    },
    "local",
  );
}

function buildSidebarLocalActions(set: ImmerSet, get: () => SidebarActionState) {
  return {
    waitForSidebarViewSync: (_workspaceId: string) =>
      sidebarSettingsQueues.get(set) ?? Promise.resolve(),
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
          active_view_id: after.activeViewId,
          draft: after.draft ? toApiSidebarDraft(after.draft) : null,
        },
        "local",
      );
    },
    updateSidebarDraft: (patch: SidebarDraftPatch) => updateSidebarDraft(set, get, patch),
    discardSidebarDraft: () => {
      if (!get().sidebarViews.draft) return;
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
          active_view_id: after.activeViewId,
          draft: null,
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

function createSidebarView(
  set: ImmerSet,
  get: () => SidebarActionState,
  grouping?: SidebarPluginGroupingChoice,
) {
  let createdViewId: string | null = null;
  mutateViews(set, get, (s) => {
    if (s.draft || (grouping && !isSidebarPluginGrouping(grouping.reference))) return false;
    const existing = grouping ? findGeneratedSidebarPluginView(s.views, grouping) : undefined;
    if (existing) {
      s.activeViewId = existing.id;
      createdViewId = existing.id;
      return;
    }
    if (s.views.length >= MAX_SIDEBAR_VIEWS) return false;
    const view = grouping
      ? createSidebarPluginView(s.views, grouping, makeId("view"))
      : createDefaultSidebarView(makeId("view"), nextNewViewName(s.views));
    s.views.push(view);
    s.activeViewId = view.id;
    createdViewId = view.id;
  });
  return createdViewId;
}

function buildSidebarDisclosureReconciler(set: ImmerSet, get: () => SidebarActionState) {
  return (viewId: string, reference: string, expected: string[], valid: string[]) => {
    const views = get().sidebarViews.views;
    const index = views.findIndex((view) => view.id === viewId);
    const view = views[index];
    if (
      !view ||
      view.group !== reference ||
      view.collapsedGroups.length !== expected.length ||
      view.collapsedGroups.some((key, position) => key !== expected[position])
    )
      return;
    const allowed = new Set(valid);
    const kept = view.collapsedGroups.filter((key) => allowed.delete(key));
    if (kept.length === view.collapsedGroups.length) return;
    mutateViews(set, get, (s) => {
      s.views[index].collapsedGroups = kept;
    });
  };
}

function buildSidebarBackendActions(set: ImmerSet, get: () => SidebarActionState) {
  const mv = (mutate: (s: UISliceState["sidebarViews"]) => boolean | void) =>
    mutateViews(set, get, mutate);
  return {
    reconcileSidebarGroupDisclosure: buildSidebarDisclosureReconciler(set, get),
    createSidebarView: (grouping?: SidebarPluginGroupingChoice) =>
      createSidebarView(set, get, grouping),
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
        const base = s.views.find((view) => view.id === s.draft!.baseViewId);
        s.views.push({
          id: makeId("view"),
          name: name.trim() || t("sidebar:untitledView"),
          filters: s.draft.filters,
          sort: s.draft.sort,
          ...(s.draft.sortWarningCount !== undefined
            ? { sortWarningCount: s.draft.sortWarningCount }
            : {}),
          group: s.draft.group,
          groupIndent: s.draft.groupIndent,
          collapsedGroups: base?.group === s.draft.group ? [...base.collapsedGroups] : [],
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
        view.sortWarningCount = s.draft.sortWarningCount;
        view.group = s.draft.group;
        view.groupIndent = s.draft.groupIndent;
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
          ...(source.sortWarningCount !== undefined
            ? { sortWarningCount: source.sortWarningCount }
            : {}),
          group: source.group,
          groupIndent: source.groupIndent,
          collapsedGroups: [...source.collapsedGroups],
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

export function buildSidebarViewActions(
  set: (recipe: (draft: UISlice) => void) => void,
  get: () => UISlice,
) {
  type Actions = ReturnType<typeof buildSidebarLocalActions> &
    ReturnType<typeof buildSidebarBackendActions>;
  const byWorkspace = new Map<string, Actions>();
  function forWorkspace(workspaceId: string): Actions {
    const existing = byWorkspace.get(workspaceId);
    if (existing) return existing;
    const scopedSet: ImmerSet = (recipe) =>
      set((state) => {
        state.sidebarViewsByWorkspace[workspaceId] ??= createSidebarWorkspaceState();
        recipe({ sidebarViews: state.sidebarViewsByWorkspace[workspaceId] });
      });
    const scopedGet = () => ({
      sidebarViews: get().sidebarViewsByWorkspace[workspaceId] ?? createSidebarWorkspaceState(),
    });
    workspaceBySetter.set(scopedSet, workspaceId);
    const actions = {
      ...buildSidebarLocalActions(scopedSet, scopedGet),
      ...buildSidebarBackendActions(scopedSet, scopedGet),
    };
    byWorkspace.set(workspaceId, actions);
    return actions;
  }
  // Every public action resolves its scope at invocation; queued callbacks retain that scope.
  const names = Object.keys({
    ...buildSidebarLocalActions(
      () => {},
      () => get(),
    ),
    ...buildSidebarBackendActions(
      () => {},
      () => get(),
    ),
  }) as (keyof Actions)[];
  return Object.fromEntries(
    names.map((name) => [
      name,
      (...args: unknown[]) => {
        let explicitScope: unknown;
        if (name === "waitForSidebarViewSync") explicitScope = args[0];
        else if (name === "reconcileSidebarGroupDisclosure") explicitScope = args[4];
        const workspaceId =
          typeof explicitScope === "string"
            ? explicitScope
            : (get() as UISlice & WorkspaceSliceState).workspaces?.activeId;
        if (!workspaceId) return name === "createSidebarView" ? null : undefined;
        if (name === "clearSidebarSyncError" && args[0] !== undefined && args[0] !== workspaceId) {
          return;
        }
        const action = forWorkspace(workspaceId)[name] as (...values: unknown[]) => unknown;
        return action(...args);
      },
    ]),
  ) as Actions;
}
