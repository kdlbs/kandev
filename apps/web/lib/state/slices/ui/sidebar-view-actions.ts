import { moveSidebarView, type SidebarViewMove } from "./sidebar-view-move";
import { createSidebarWorkspaceState } from "./sidebar-workspace-state";
import { produce } from "immer";
import type { WorkspaceSliceState } from "../workspace/types";
import type { SidebarDraftSaveApi, SidebarViewApi } from "@/lib/types/http-user-settings";
import type { UISlice, UISliceState } from "./types";
import type {
  FilterClause,
  GroupKey,
  SidebarDisclosureContext,
  SidebarView,
  SidebarViewDraft,
  SidebarViewStateSetter as ImmerSet,
} from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";
import {
  rebaseSidebarMutation,
  sidebarWriteDelta,
  type SidebarSnapshot,
} from "./sidebar-view-delta";
import { sidebarViewDisclosure } from "./sidebar-view-disclosure";
import { prepareSidebarDraftSave } from "./sidebar-draft-save";
import { rollbackSidebarState, restoreFailedViewMutation } from "./sidebar-view-rollback";
import { createDefaultSidebarView, MAX_SIDEBAR_VIEWS } from "./sidebar-view-builtins";
import { t } from "@/lib/i18n";
import {
  clearSidebarWriteRollback,
  getSidebarWriteJournal,
  pruneSidebarFailures,
  recordSidebarWriteFailure,
  settleSidebarFailures,
  restoreSidebarSnapshot,
  snapshotSidebar,
  type SidebarWriteKind,
} from "./sidebar-view-journal";
import {
  admitDeferredSidebarState,
  bindSidebarViewSettingsWorkspace,
  enqueueSidebarSettingsSync,
  trackSidebarSettingsWrite,
  waitForSidebarSettings,
} from "./sidebar-view-settings-sync";
import {
  createSidebarPluginView,
  findGeneratedSidebarPluginView,
  isSidebarPluginGrouping,
  type SidebarPluginGroupingChoice,
} from "@/lib/sidebar/sidebar-plugin-grouping";

type SidebarActionState = Pick<UISliceState, "sidebarViews">;
type SidebarWriteOptions = {
  kind: SidebarWriteKind;
  admit?: (snapshot: SidebarSnapshot) => boolean;
  selection?: SidebarViewApi;
  saveDraft?: SidebarDraftSaveApi;
  move?: SidebarViewMove;
};
function makeId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function nextNewViewName(views: SidebarView[]): string {
  const names = new Set(views.map((view) => view.name));
  const base = t("sidebar:newView");
  if (!names.has(base)) return base;
  let suffix = 2;
  while (names.has(t("sidebar:newViewNumbered", { suffix }))) suffix += 1;
  return t("sidebar:newViewNumbered", { suffix });
}

function syncSidebarWrite(
  set: ImmerSet,
  get: () => SidebarActionState,
  before: SidebarSnapshot,
  after: SidebarSnapshot,
  { kind, admit, selection, saveDraft, move }: SidebarWriteOptions,
) {
  const journal = getSidebarWriteJournal(set);
  if (journal.pending === 0) journal.confirmed = before;
  journal.pending++;
  const thisRequestId = ++journal.latestRequestId;
  set((state) => {
    state.sidebarViews.syncPending = true;
    if (kind === "selection" || kind === "draft_save") state.sidebarViews.transitionPending = true;
  });
  let desired = after;
  let succeeded = false;
  const writePayload = () => {
    admitDeferredSidebarState(journal, get);
    pruneSidebarFailures(journal);
    if (admit && !admit(journal.confirmed!)) return null;
    let rebased = journal.confirmed!;
    if (kind !== "local")
      for (const failed of journal.failedViewMutations ?? [])
        rebased = rebaseSidebarMutation(rebased, failed.before, failed.after, failed.move);
    desired = rebaseSidebarMutation(rebased, before, after, move);
    if (saveDraft) return { save_draft: saveDraft };
    const patch = sidebarWriteDelta(
      before,
      after,
      kind !== "local" ? journal.failedViewMutations : undefined,
      desired,
    );
    if (kind === "compaction" && patch.draft_update) patch.draft_update.require_existing = true;
    if (patch.view_order) patch.view_order_base = journal.confirmed!.views.map((view) => view.id);
    return { ...patch, ...(selection ? { select_plugin_grouping: selection } : {}) };
  };
  const completed = enqueueSidebarSettingsSync(set, get, writePayload, () => desired, kind).then(
    (persisted) => {
      succeeded = true;
      journal.pending--;
      if (!persisted && thisRequestId !== journal.latestRequestId) return;
      const failedUnreplayedWrite = settleSidebarFailures(journal, kind, persisted, thisRequestId);
      if (
        thisRequestId === journal.latestRequestId &&
        journal.failedWriteKind === "views" &&
        kind === "local" &&
        journal.failedRollback
      ) {
        restoreFailedViewMutation(set, journal.failedRollback);
      }
      if (!failedUnreplayedWrite || thisRequestId === journal.latestRequestId)
        clearSidebarWriteRollback(journal);
      if (thisRequestId === journal.latestRequestId)
        set((state) => {
          const server = state.sidebarViews.deferredServerState;
          if (server) restoreSidebarSnapshot(state.sidebarViews, server);
          else if (kind === "compaction" || failedUnreplayedWrite)
            Object.assign(state.sidebarViews, journal.confirmed!);
          state.sidebarViews.syncPending = false;
          state.sidebarViews.transitionPending = false;
          if (persisted && kind !== "local" && !failedUnreplayedWrite)
            state.sidebarViews.syncError = null;
          state.sidebarViews.deferredServerState = null;
        });
    },
    (err) => {
      const rollback = recordSidebarWriteFailure(journal, { before, after, move }, kind, err);
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
        state.sidebarViews.transitionPending = false;
        state.sidebarViews.deferredServerState = null;
      });
      clearSidebarWriteRollback(journal);
      journal.failedViewMutations = undefined;
    },
  );
  trackSidebarSettingsWrite(set, completed);
  return completed.then(() => succeeded);
}

function mutateViews(
  set: ImmerSet,
  get: () => SidebarActionState,
  mutate: (slice: UISliceState["sidebarViews"]) => boolean | void,
  { kind = "views", ...options }: Partial<SidebarWriteOptions> = {},
): Promise<boolean> {
  const snapshot = snapshotSidebar(get().sidebarViews);
  let committed = false;
  let after: UISliceState["sidebarViews"];
  if (kind === "compaction") {
    // Compaction changes query identity only after persistence acknowledges it.
    after = produce(get().sidebarViews, (draft) => {
      committed = mutate(draft) !== false;
    });
  } else {
    set((draft) => {
      committed = mutate(draft.sidebarViews) !== false;
    });
    after = get().sidebarViews;
  }
  if (!committed) return Promise.resolve(true);
  const afterSnapshot = snapshotSidebar(after);
  return syncSidebarWrite(set, get, snapshot, afterSnapshot, { kind, ...options });
}

function saveSidebarDraft(
  set: ImmerSet,
  get: () => SidebarActionState,
  name?: string,
  owner?: Pick<SidebarViewDraft, "baseViewId" | "group">,
) {
  const state = get().sidebarViews;
  if (state.syncPending) return Promise.resolve(false);
  if (owner && (state.draft?.baseViewId !== owner.baseViewId || state.draft.group !== owner.group))
    return Promise.resolve(false);
  const target = name === undefined ? state.draft?.baseViewId : makeId("view");
  if (!target) return Promise.resolve(false);
  const prepared = prepareSidebarDraftSave(
    state,
    target,
    name?.trim() || t("sidebar:untitledView"),
  );
  if (!prepared) return Promise.resolve(false);
  return mutateViews(
    set,
    get,
    (slice) => {
      const index = slice.views.findIndex((view) => view.id === target);
      if (index < 0) slice.views.push(prepared.view);
      else slice.views[index] = prepared.view;
      slice.activeViewId = target;
      slice.draft = null;
    },
    { kind: "draft_save", saveDraft: prepared.command },
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
    const group = patch.group ?? current.group;
    let collapsedGroups: string[] | undefined;
    if (group !== active.group) {
      collapsedGroups = group === current.group ? (current.collapsedGroups ?? []) : [];
    }
    const next: SidebarViewDraft = {
      baseViewId: active.id,
      filters: patch.filters ?? current.filters,
      sort: patch.sort ?? current.sort,
      ...(current.sortWarningCount !== undefined
        ? { sortWarningCount: current.sortWarningCount }
        : {}),
      group,
      groupIndent: patch.groupIndent ?? current.groupIndent,
      ...(collapsedGroups ? { collapsedGroups } : {}),
      taskRow: cloneSidebarTaskRowPresentation(patch.taskRow ?? current.taskRow),
    };
    draft.sidebarViews.draft = next;
  });
  if (!committed) return;
  const after = snapshotSidebar(get().sidebarViews);
  syncSidebarWrite(set, get, before, after, { kind: "local" });
}

function buildSidebarLocalActions(set: ImmerSet, get: () => SidebarActionState) {
  return {
    waitForSidebarViewSync: (_workspaceId: string) => waitForSidebarSettings(set),
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
      syncSidebarWrite(set, get, before, after, { kind: "local" });
    },
    updateSidebarDraft: (patch: SidebarDraftPatch) => updateSidebarDraft(set, get, patch),
    discardSidebarDraft: () => {
      if (!get().sidebarViews.draft) return;
      const before = snapshotSidebar(get().sidebarViews);
      set((draft) => {
        draft.sidebarViews.draft = null;
      });
      const after = snapshotSidebar(get().sidebarViews);
      syncSidebarWrite(set, get, before, after, { kind: "local" });
    },
    clearSidebarSyncError: () =>
      set((draft) => {
        draft.sidebarViews.syncError = null;
      }),
  };
}

function createSidebarView(set: ImmerSet, get: () => SidebarActionState) {
  let createdViewId: string | null = null;
  mutateViews(
    set,
    get,
    (s) => {
      if (s.draft) return false;
      if (s.views.length >= MAX_SIDEBAR_VIEWS) return false;
      const view = createDefaultSidebarView(makeId("view"), nextNewViewName(s.views));
      s.views.push(view);
      s.activeViewId = view.id;
      createdViewId = view.id;
    },
    { kind: "create" },
  );
  return createdViewId;
}

function selectSidebarPluginGrouping(
  set: ImmerSet,
  get: () => SidebarActionState,
  grouping: SidebarPluginGroupingChoice,
): Promise<boolean> {
  const state = get().sidebarViews;
  if (
    state.draft ||
    !isSidebarPluginGrouping(grouping.reference) ||
    (state.views.length >= MAX_SIDEBAR_VIEWS &&
      !findGeneratedSidebarPluginView(state.views, grouping))
  )
    return Promise.resolve(false);
  const before = snapshotSidebar(state);
  const candidate = createSidebarPluginView(grouping, makeId("view"));
  set((draft) => {
    draft.sidebarViews.syncError = null;
  });
  return syncSidebarWrite(set, get, before, before, {
    kind: "selection",
    selection: toApiSidebarView(candidate),
  });
}

function buildSidebarDisclosureReconciler(set: ImmerSet, get: () => SidebarActionState) {
  return (
    viewId: string,
    reference: string,
    expected: string[],
    valid: string[],
    context?: SidebarDisclosureContext,
  ) => {
    const state = get().sidebarViews;
    const index = state.views.findIndex((view) => view.id === viewId);
    const view = state.views[index];
    if (!view) return Promise.resolve(true);
    const draft = state.draft?.baseViewId === viewId ? state.draft : null;
    const keys = sidebarViewDisclosure(view, draft);
    const draftOwned = Boolean(draft && draft.group !== view.group);
    if (
      (draft?.group ?? view.group) !== reference ||
      keys.length !== expected.length ||
      keys.some((key, position) => key !== expected[position])
    )
      return Promise.resolve(true);
    const allowed = new Set(valid);
    const kept = keys.filter((key) => allowed.delete(key));
    if (kept.length === keys.length) return Promise.resolve(true);
    const admit = (current: SidebarSnapshot) => {
      if (context?.isCurrent && !context.isCurrent()) return false;
      const saved = current.views.find((candidate) => candidate.id === viewId);
      if (!saved) return false;
      const preview = current.draft?.baseViewId === viewId ? current.draft : null;
      const disclosure = sidebarViewDisclosure(saved, preview);
      return (
        Boolean(preview && preview.group !== saved.group) === draftOwned &&
        (preview?.group ?? saved.group) === reference &&
        disclosure.length === expected.length &&
        disclosure.every((key, position) => key === expected[position])
      );
    };
    return mutateViews(
      set,
      get,
      (s) => {
        const owner =
          s.draft?.baseViewId === viewId && s.draft.group !== view.group ? s.draft : s.views[index];
        owner.collapsedGroups = kept;
      },
      { kind: "compaction", admit },
    );
  };
}

function buildSidebarBackendActions(set: ImmerSet, get: () => SidebarActionState) {
  const mv = (
    mutate: (s: UISliceState["sidebarViews"]) => boolean | void,
    options: Partial<SidebarWriteOptions> = {},
  ) => mutateViews(set, get, mutate, options);
  return {
    reconcileSidebarGroupDisclosure: buildSidebarDisclosureReconciler(set, get),
    createSidebarView: () => createSidebarView(set, get),
    selectSidebarPluginGrouping: (grouping: SidebarPluginGroupingChoice) =>
      selectSidebarPluginGrouping(set, get, grouping),
    toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) =>
      mv((s) => {
        const view = s.views.find((v) => v.id === viewId);
        if (!view) return false;
        const owner =
          s.draft?.baseViewId === viewId && s.draft.group !== view.group ? s.draft : view;
        const keys = (owner.collapsedGroups ??= []);
        const idx = keys.indexOf(groupKey);
        if (idx === -1) keys.push(groupKey);
        else keys.splice(idx, 1);
      }),
    saveSidebarDraftAs: (name: string, owner: Pick<SidebarViewDraft, "baseViewId" | "group">) =>
      saveSidebarDraft(set, get, name, owner),
    saveSidebarDraftOverwrite: () => saveSidebarDraft(set, get),
    duplicateSidebarView: (viewId: string, name: string) =>
      mv(
        (s) => {
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
        },
        { kind: "create" },
      ),
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
    reorderSidebarViews: (id: string, overId: string) => {
      const views = get().sidebarViews.views;
      const move = {
        id,
        overId,
        after:
          views.findIndex((view) => view.id === id) < views.findIndex((view) => view.id === overId),
      };
      return mutateViews(
        set,
        get,
        (state) => {
          const reordered = moveSidebarView(state.views, move);
          if (!reordered) return false;
          state.views = reordered;
        },
        { move },
      );
    },
  };
}

function unavailableSidebarAction(name: string) {
  if (name === "createSidebarView") return null;
  if (name === "selectSidebarPluginGrouping" || name === "reconcileSidebarGroupDisclosure")
    return Promise.resolve(false);
}

const TRANSITION_SAFE_ACTIONS = new Set([
  "waitForSidebarViewSync",
  "clearSidebarSyncError",
  "reconcileSidebarGroupDisclosure",
]);

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
    bindSidebarViewSettingsWorkspace(scopedSet, workspaceId);
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
        else if (name === "reconcileSidebarGroupDisclosure")
          explicitScope = (args[4] as SidebarDisclosureContext | undefined)?.workspaceId;
        const workspaceId =
          typeof explicitScope === "string"
            ? explicitScope
            : (get() as UISlice & WorkspaceSliceState).workspaces?.activeId;
        if (!workspaceId) return unavailableSidebarAction(name);
        if (
          get().sidebarViewsByWorkspace[workspaceId]?.transitionPending &&
          !TRANSITION_SAFE_ACTIONS.has(name)
        )
          return unavailableSidebarAction(name);
        if (name === "clearSidebarSyncError" && args[0] !== undefined && args[0] !== workspaceId) {
          return;
        }
        const action = forWorkspace(workspaceId)[name] as (...values: unknown[]) => unknown;
        return action(...args);
      },
    ]),
  ) as Actions;
}
