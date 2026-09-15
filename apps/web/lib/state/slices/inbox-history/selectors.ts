import type { AppState } from "@/lib/state/store";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";
import type { InboxHistoryWorkspaceState } from "./types";

const EMPTY_BUNDLES: InboxHistoryBundle[] = [];

const EMPTY_WORKSPACE_STATE: InboxHistoryWorkspaceState = {
  bundles: EMPTY_BUNDLES,
  total: 0,
  hasMore: false,
  status: "idle",
  appliedGeneration: 0,
};

function selectActiveWorkspaceState(state: AppState): InboxHistoryWorkspaceState {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_WORKSPACE_STATE;
  return state.inboxHistory.byWorkspaceId[workspaceId] ?? EMPTY_WORKSPACE_STATE;
}

/** The History tab's own bundle count (AC .16), unrelated to and never
 * reading the sidebar/Needs-you count state (AC .17). */
export function selectInboxHistoryCount(state: AppState): number {
  return selectActiveWorkspaceState(state).total;
}

export function selectInboxHistoryBundles(state: AppState): InboxHistoryBundle[] {
  return selectActiveWorkspaceState(state).bundles;
}

export function selectInboxHistoryStatus(state: AppState) {
  return selectActiveWorkspaceState(state).status;
}

export function selectInboxHistoryHasMore(state: AppState): boolean {
  return selectActiveWorkspaceState(state).hasMore;
}
