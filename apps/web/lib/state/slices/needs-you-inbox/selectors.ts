import type { AppState } from "@/lib/state/store";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import type { NeedsYouInboxWorkspaceState } from "./types";

const EMPTY_BUNDLES: ClarificationInboxBundle[] = [];

const EMPTY_WORKSPACE_STATE: NeedsYouInboxWorkspaceState = {
  bundles: EMPTY_BUNDLES,
  count: 0,
  hiddenCount: 0,
  nextSnoozeExpiry: null,
  hasMore: false,
  status: "idle",
  appliedGeneration: 0,
};

function selectActiveWorkspaceState(state: AppState): NeedsYouInboxWorkspaceState {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_WORKSPACE_STATE;
  return state.needsYouInbox.byWorkspaceId[workspaceId] ?? EMPTY_WORKSPACE_STATE;
}

/** The sidebar badge count for the active workspace. Absent workspace or
 * absent state renders no badge, not a zero -- distinguishing "not seeded"
 * from "confirmed empty" at the presentation layer (callers gate visibility
 * separately). */
export function selectNeedsYouInboxCount(state: AppState): number {
  return selectActiveWorkspaceState(state).count;
}

export function selectNeedsYouInboxBundles(state: AppState): ClarificationInboxBundle[] {
  return selectActiveWorkspaceState(state).bundles;
}

export function selectNeedsYouInboxStatus(state: AppState) {
  return selectActiveWorkspaceState(state).status;
}

export function selectNeedsYouInboxHiddenCount(state: AppState): number {
  return selectActiveWorkspaceState(state).hiddenCount;
}

export function selectNeedsYouInboxRevision(state: AppState): number {
  return selectActiveWorkspaceState(state).appliedGeneration;
}

export function selectNeedsYouInboxHasMore(state: AppState): boolean {
  return selectActiveWorkspaceState(state).hasMore;
}

export function selectNeedsYouInboxNextSnoozeExpiry(state: AppState): string | null {
  return selectActiveWorkspaceState(state).nextSnoozeExpiry;
}
