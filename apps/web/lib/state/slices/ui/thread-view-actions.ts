import { t } from "@/lib/i18n";
import { generateUUID } from "@/lib/utils";
import { updateUserSettingsWithRetry } from "@/lib/user-settings-sync";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";
import { createDefaultThreadView, MAX_THREAD_VIEWS } from "./thread-view-builtins";
import type { UISlice, UISliceState } from "./types";
import type {
  ThreadFilterClause,
  ThreadSortSpec,
  ThreadTaskScope,
  ThreadView,
  ThreadViewDraft,
  ThreadViewSnapshot,
} from "./thread-view-types";
import { toApiThreadDraft, toApiThreadView } from "./thread-view-wire";

type ImmerSet = (recipe: (draft: UISlice) => void, shouldReplace?: false | undefined) => void;

type ThreadViewWriteJournal = {
  latestRequestId: number;
  failedRollback?: ThreadViewSnapshot;
  failedRetry?: {
    snapshot: ThreadViewSnapshot;
    payload: UserSettingsUpdatePayload;
  };
};

const threadSettingsQueues = new WeakMap<ImmerSet, Promise<void>>();
const threadWriteJournals = new WeakMap<ImmerSet, ThreadViewWriteJournal>();

function makeId(prefix: string): string {
  return `${prefix}-${generateUUID()}`;
}

function nextNewViewName(views: ThreadView[]): string {
  const base = t("threads:newView");
  const names = new Set(views.map((view) => view.name));
  if (!names.has(base)) return base;
  let suffix = 2;
  while (names.has(t("threads:newViewNumbered", { suffix }))) suffix += 1;
  return t("threads:newViewNumbered", { suffix });
}

function cloneScope(scope: ThreadTaskScope): ThreadTaskScope {
  return { mode: scope.mode, taskIds: [...scope.taskIds] } as ThreadTaskScope;
}

function cloneClause(clause: ThreadFilterClause): ThreadFilterClause {
  return { ...clause, value: Array.isArray(clause.value) ? [...clause.value] : clause.value };
}

function cloneView(view: ThreadView): ThreadView {
  return {
    id: view.id,
    name: view.name,
    taskScope: cloneScope(view.taskScope),
    filters: view.filters.map(cloneClause),
    sort: { ...view.sort },
    maxColumns: view.maxColumns,
  };
}

function cloneDraft(draft: ThreadViewDraft | null): ThreadViewDraft | null {
  if (!draft) return null;
  return {
    baseViewId: draft.baseViewId,
    taskScope: cloneScope(draft.taskScope),
    filters: draft.filters.map(cloneClause),
    sort: { ...draft.sort },
    maxColumns: draft.maxColumns,
  };
}

function snapshotThreadViews(state: UISliceState["threadViews"]): ThreadViewSnapshot {
  return {
    views: state.views.map(cloneView),
    activeViewId: state.activeViewId,
    draft: cloneDraft(state.draft),
  };
}

function cloneSnapshot(snapshot: ThreadViewSnapshot): ThreadViewSnapshot {
  return {
    views: snapshot.views.map(cloneView),
    activeViewId: snapshot.activeViewId,
    draft: cloneDraft(snapshot.draft),
  };
}

function toThreadSettingsPayload(
  state: ThreadViewSnapshot | UISliceState["threadViews"],
): UserSettingsUpdatePayload {
  return {
    thread_views: state.views.map(toApiThreadView),
    thread_active_view_id: state.activeViewId,
    thread_view_draft: state.draft ? toApiThreadDraft(state.draft) : null,
  };
}

function getJournal(set: ImmerSet): ThreadViewWriteJournal {
  const existing = threadWriteJournals.get(set);
  if (existing) return existing;
  const created = { latestRequestId: 0 };
  threadWriteJournals.set(set, created);
  return created;
}

function enqueueThreadSettingsSync(
  set: ImmerSet,
  payload: UserSettingsUpdatePayload,
): Promise<void> {
  const previous = threadSettingsQueues.get(set);
  const request = previous
    ? previous.then(() => updateUserSettingsWithRetry(payload))
    : updateUserSettingsWithRetry(payload);
  threadSettingsQueues.set(
    set,
    request.catch(() => undefined),
  );
  return request;
}

function restoreThreadSnapshot(set: ImmerSet, snapshot: ThreadViewSnapshot, error?: unknown): void {
  set((draft) => {
    draft.threadViews.views = snapshot.views.map(cloneView);
    draft.threadViews.activeViewId = snapshot.activeViewId;
    draft.threadViews.draft = cloneDraft(snapshot.draft);
    draft.threadViews.deferredServerState = null;
    draft.threadViews.syncPending = false;
    if (error !== undefined) {
      draft.threadViews.syncError =
        error instanceof Error ? error.message : t("threads:failedToSyncViews");
    }
  });
}

function syncThreadWrite(
  set: ImmerSet,
  get: () => UISlice,
  before: ThreadViewSnapshot,
  payload: UserSettingsUpdatePayload,
  after: ThreadViewSnapshot,
): void {
  const journal = getJournal(set);
  const requestId = ++journal.latestRequestId;
  enqueueThreadSettingsSync(set, payload).then(
    () => {
      if (requestId !== journal.latestRequestId) return;
      journal.failedRollback = undefined;
      journal.failedRetry = undefined;
      set((draft) => {
        draft.threadViews.syncPending = false;
        draft.threadViews.syncError = null;
        draft.threadViews.deferredServerState = null;
      });
    },
    (error) => {
      journal.failedRollback ??= before;
      if (requestId !== journal.latestRequestId) {
        set((draft) => {
          draft.threadViews.syncError =
            error instanceof Error ? error.message : t("threads:failedToSyncViews");
        });
        return;
      }
      const rollback = journal.failedRollback;
      const deferredServerState = get().threadViews.deferredServerState;
      journal.failedRollback = undefined;
      journal.failedRetry = { snapshot: cloneSnapshot(after), payload };
      if (deferredServerState) {
        restoreThreadSnapshot(set, deferredServerState, error);
      } else if (rollback) {
        restoreThreadSnapshot(set, rollback, error);
      }
    },
  );
}

function mutateThreadViews(
  set: ImmerSet,
  get: () => UISlice,
  mutate: (state: UISliceState["threadViews"]) => boolean | void,
): void {
  const before = snapshotThreadViews(get().threadViews);
  let committed = false;
  set((draft) => {
    committed = mutate(draft.threadViews) !== false;
    if (committed) {
      draft.threadViews.syncPending = true;
      draft.threadViews.syncError = null;
    }
  });
  if (!committed) return;
  getJournal(set).failedRetry = undefined;
  syncThreadWrite(
    set,
    get,
    before,
    toThreadSettingsPayload(get().threadViews),
    snapshotThreadViews(get().threadViews),
  );
}

// eslint-disable-next-line max-lines-per-function -- Keeps the saved-view action family and its serialized write path together.
export function buildThreadViewActions(set: ImmerSet, get: () => UISlice) {
  const mutate = (fn: (state: UISliceState["threadViews"]) => boolean | void) =>
    mutateThreadViews(set, get, fn);

  return {
    setThreadActiveView: (viewId: string) => {
      const before = snapshotThreadViews(get().threadViews);
      let committed = false;
      set((draft) => {
        if (!draft.threadViews.views.some((view) => view.id === viewId)) return;
        draft.threadViews.activeViewId = viewId;
        draft.threadViews.draft = null;
        draft.threadViews.syncPending = true;
        draft.threadViews.syncError = null;
        committed = true;
      });
      if (committed) {
        getJournal(set).failedRetry = undefined;
        syncThreadWrite(
          set,
          get,
          before,
          toThreadSettingsPayload(get().threadViews),
          snapshotThreadViews(get().threadViews),
        );
      }
    },
    createThreadView: () => {
      let createdId: string | null = null;
      mutate((state) => {
        if (state.views.length >= MAX_THREAD_VIEWS || state.draft) return false;
        const view = createDefaultThreadView(makeId("view"), nextNewViewName(state.views));
        state.views.push(view);
        state.activeViewId = view.id;
        createdId = view.id;
      });
      return createdId;
    },
    updateThreadViewDraft: (
      patch: Partial<{
        taskScope: ThreadTaskScope;
        filters: ThreadFilterClause[];
        sort: ThreadSortSpec;
        maxColumns: number | null;
      }>,
    ) => {
      mutate((state) => {
        const active = state.views.find((view) => view.id === state.activeViewId);
        if (!active) return false;
        const current = state.draft ?? {
          baseViewId: active.id,
          taskScope: cloneScope(active.taskScope),
          filters: active.filters.map(cloneClause),
          sort: { ...active.sort },
          maxColumns: active.maxColumns,
        };
        state.draft = {
          baseViewId: active.id,
          taskScope: patch.taskScope ? cloneScope(patch.taskScope) : cloneScope(current.taskScope),
          filters: patch.filters
            ? patch.filters.map(cloneClause)
            : current.filters.map(cloneClause),
          sort: patch.sort ? { ...patch.sort } : { ...current.sort },
          maxColumns: patch.maxColumns === undefined ? current.maxColumns : patch.maxColumns,
        };
      });
    },
    saveThreadViewDraftAs: (name: string) =>
      mutate((state) => {
        if (!state.draft || state.views.length >= MAX_THREAD_VIEWS) return false;
        if (
          state.draft.taskScope.mode === "selected" &&
          state.draft.taskScope.taskIds.length === 0
        ) {
          return false;
        }
        const view: ThreadView = {
          id: makeId("view"),
          name: name.trim() || t("threads:untitledView"),
          taskScope: cloneScope(state.draft.taskScope),
          filters: state.draft.filters.map(cloneClause),
          sort: { ...state.draft.sort },
          maxColumns: state.draft.maxColumns,
        };
        state.views.push(view);
        state.activeViewId = view.id;
        state.draft = null;
      }),
    saveThreadViewDraftOverwrite: () =>
      mutate((state) => {
        if (!state.draft) return false;
        if (
          state.draft.taskScope.mode === "selected" &&
          state.draft.taskScope.taskIds.length === 0
        ) {
          return false;
        }
        const view = state.views.find((candidate) => candidate.id === state.draft?.baseViewId);
        if (!view) return false;
        view.taskScope = cloneScope(state.draft.taskScope);
        view.filters = state.draft.filters.map(cloneClause);
        view.sort = { ...state.draft.sort };
        view.maxColumns = state.draft.maxColumns;
        state.draft = null;
      }),
    discardThreadViewDraft: () =>
      mutate((state) => {
        if (!state.draft) return false;
        state.draft = null;
      }),
    deleteThreadView: (viewId: string) =>
      mutate((state) => {
        if (state.views.length <= 1 || !state.views.some((view) => view.id === viewId))
          return false;
        state.views = state.views.filter((view) => view.id !== viewId);
        if (state.activeViewId === viewId) state.activeViewId = state.views[0].id;
        state.draft = null;
      }),
    renameThreadView: (viewId: string, name: string) =>
      mutate((state) => {
        const view = state.views.find((candidate) => candidate.id === viewId);
        const next = name.trim();
        if (!view || !next || next === view.name) return false;
        view.name = next;
      }),
    duplicateThreadView: (viewId: string, name: string) =>
      mutate((state) => {
        const source = state.views.find((view) => view.id === viewId);
        if (!source || state.views.length >= MAX_THREAD_VIEWS) return false;
        const copy = cloneView(source);
        copy.id = makeId("view");
        copy.name = name.trim() || t("threads:copyName", { name: source.name });
        copy.filters = copy.filters.map((filter) => ({ ...filter, id: makeId("clause") }));
        state.views.push(copy);
        state.activeViewId = copy.id;
        state.draft = null;
      }),
    reapplyThreadViewSort: () =>
      set((draft) => {
        draft.threadViews.orderResetGeneration += 1;
      }),
    retryThreadViewSync: () => {
      const journal = getJournal(set);
      const retry = journal.failedRetry;
      if (!retry || get().threadViews.syncPending) return;
      const before = snapshotThreadViews(get().threadViews);
      set((draft) => {
        draft.threadViews.views = retry.snapshot.views.map(cloneView);
        draft.threadViews.activeViewId = retry.snapshot.activeViewId;
        draft.threadViews.draft = cloneDraft(retry.snapshot.draft);
        draft.threadViews.syncError = null;
        draft.threadViews.syncPending = true;
        draft.threadViews.deferredServerState = null;
      });
      syncThreadWrite(set, get, before, retry.payload, retry.snapshot);
    },
    clearThreadViewSyncError: () =>
      set((draft) => {
        draft.threadViews.syncError = null;
      }),
  };
}
