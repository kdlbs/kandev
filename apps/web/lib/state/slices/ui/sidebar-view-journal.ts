import { ApiError } from "@/lib/api/client";
import type { SidebarViewMove } from "./sidebar-view-move";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { cloneDraft, cloneView } from "./sidebar-view-clone";
import type { SidebarSnapshot } from "./sidebar-view-delta";
import type { SidebarSliceState } from "./sidebar-view-types";

export type SidebarWriteKind =
  | "views"
  | "create"
  | "local"
  | "compaction"
  | "selection"
  | "draft_save";
type SidebarViewMutation = {
  before: SidebarSnapshot;
  after: SidebarSnapshot;
  move?: SidebarViewMove;
};
export type SidebarWriteJournal = {
  latestRequestId: number;
  pending: number;
  confirmed?: SidebarSnapshot;
  failedRollback?: SidebarSnapshot;
  failedWriteKind?: SidebarWriteKind;
  failedUnreplayedWrite?: boolean;
  failedViewMutations?: SidebarViewMutation[];
  reconcileUncertainCreate?: boolean;
};

const journals = new WeakMap<object, SidebarWriteJournal>();

export function getSidebarWriteJournal(owner: object): SidebarWriteJournal {
  const existing = journals.get(owner);
  if (existing) return existing;
  const created: SidebarWriteJournal = { latestRequestId: 0, pending: 0 };
  journals.set(owner, created);
  return created;
}

export function clearSidebarWriteRollback(journal: SidebarWriteJournal): void {
  journal.failedRollback = undefined;
  journal.failedWriteKind = undefined;
  journal.failedUnreplayedWrite = false;
}

function isUnreplayedSidebarWrite(kind: SidebarWriteKind): boolean {
  return kind !== "views" && kind !== "compaction";
}

export function snapshotSidebar(state: SidebarSliceState): SidebarSnapshot {
  return {
    views: state.views.map(cloneView),
    activeViewId: state.activeViewId,
    draft: cloneDraft(state.draft),
    serverRevision: state.serverRevision,
  };
}

export function admitSidebarServerSnapshot(
  journal: SidebarWriteJournal,
  snapshot: SidebarSnapshot,
) {
  const order = compareUserSettingsRevisions(
    snapshot.serverRevision,
    journal.confirmed?.serverRevision,
  );
  if (order === null || order >= 0) journal.confirmed = snapshot;
}

export function replayableSidebarMove(journal: SidebarWriteJournal, move?: SidebarViewMove) {
  return (
    !move ||
    [move.id, move.overId].every((id) => journal.confirmed?.views.some((view) => view.id === id))
  );
}

function shouldPreserveIndependentMutation(
  journal: SidebarWriteJournal,
  mutation: SidebarViewMutation,
  kind: SidebarWriteKind,
  rejected: boolean,
) {
  if (!rejected || kind !== "views" || mutation.move) return false;
  return Boolean(journal.failedViewMutations?.some((failedMutation) => failedMutation.move));
}

export function recordSidebarWriteFailure(
  journal: SidebarWriteJournal,
  mutation: SidebarViewMutation,
  kind: SidebarWriteKind,
  error: unknown,
) {
  const { before } = mutation;
  journal.pending--;
  if (isUnreplayedSidebarWrite(kind)) journal.failedUnreplayedWrite = true;
  const rejected =
    error instanceof ApiError &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408 &&
    error.status !== 429;
  const preserveIndependentMutation = shouldPreserveIndependentMutation(
    journal,
    mutation,
    kind,
    rejected,
  );
  if (rejected) retireRejectedSidebarMutations(journal);
  if (kind === "create" && !rejected) journal.reconcileUncertainCreate = true;
  recordFailedViewIntent(journal, mutation, kind, rejected && !preserveIndependentMutation);
  const rollback = journal.failedRollback ?? before;
  journal.failedRollback = rollback;
  journal.failedWriteKind ??= kind;
  return rollback;
}

export function restoreSidebarSnapshot(state: SidebarSliceState, snapshot: SidebarSnapshot) {
  const error = state.syncError;
  Object.assign(state, snapshot);
  state.syncError = error;
}

function retireRejectedSidebarMutations(journal: SidebarWriteJournal) {
  const failed = journal.failedViewMutations;
  if (failed?.length) journal.failedUnreplayedWrite = true;
  journal.failedViewMutations = undefined;
}

function recordFailedViewIntent(
  journal: SidebarWriteJournal,
  mutation: SidebarViewMutation,
  kind: SidebarWriteKind,
  rejected: boolean,
) {
  if (kind !== "views") return;
  if (rejected || !replayableSidebarMove(journal, mutation.move)) {
    journal.failedUnreplayedWrite = true;
    return;
  }
  (journal.failedViewMutations ??= []).push(mutation);
}

export function settleSidebarFailures(
  journal: SidebarWriteJournal,
  kind: SidebarWriteKind,
  persisted: unknown,
  requestId: number,
) {
  const latest = requestId === journal.latestRequestId;
  if ((!persisted || (kind === "local" && latest)) && journal.failedViewMutations?.length)
    journal.failedUnreplayedWrite = true;
  if (kind !== "local" || latest) journal.failedViewMutations = undefined;
  return journal.failedUnreplayedWrite;
}

export function pruneSidebarFailures(journal: SidebarWriteJournal) {
  const failed = journal.failedViewMutations;
  const retained = failed?.filter((mutation) => replayableSidebarMove(journal, mutation.move));
  if (failed?.length !== retained?.length) journal.failedUnreplayedWrite = true;
  journal.failedViewMutations = retained;
}
