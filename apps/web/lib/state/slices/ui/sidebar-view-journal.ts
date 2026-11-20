import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { cloneDraft, cloneView } from "./sidebar-view-clone";
import type { SidebarSnapshot } from "./sidebar-view-delta";
import type { SidebarSliceState } from "./sidebar-view-types";

export type SidebarWriteKind = "views" | "local" | "compaction" | "selection";
export type SidebarWriteJournal = {
  latestRequestId: number;
  pending: number;
  confirmed?: SidebarSnapshot;
  failedRollback?: SidebarSnapshot;
  failedWriteKind?: SidebarWriteKind;
  failedViewMutations?: { before: SidebarSnapshot; after: SidebarSnapshot }[];
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
