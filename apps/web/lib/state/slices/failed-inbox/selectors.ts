import type { AppState } from "@/lib/state/store";
import type { FailedInboxRow } from "@/lib/types/failed-inbox";
import type { FailedInboxWorkspaceState } from "./types";

const EMPTY_ROWS: FailedInboxRow[] = [];

const EMPTY_WORKSPACE_STATE: FailedInboxWorkspaceState = {
  rows: EMPTY_ROWS,
  count: 0,
  truncated: false,
  status: "idle",
  appliedGeneration: 0,
};

function selectActiveWorkspaceState(state: AppState): FailedInboxWorkspaceState {
  const workspaceId = state.workspaces.activeId;
  if (!workspaceId) return EMPTY_WORKSPACE_STATE;
  return state.failedInbox.byWorkspaceId[workspaceId] ?? EMPTY_WORKSPACE_STATE;
}

export function selectFailedInboxRows(state: AppState): FailedInboxRow[] {
  return selectActiveWorkspaceState(state).rows;
}

export function selectFailedInboxStatus(state: AppState) {
  return selectActiveWorkspaceState(state).status;
}

export function selectFailedInboxTruncated(state: AppState): boolean {
  return selectActiveWorkspaceState(state).truncated;
}

/** The Failed tab's own badge count (AC-UI-INBOX-FAILED-001.16): absent
 * workspace or absent state renders no badge, not a zero -- distinguishing
 * "not read yet" from "confirmed empty" at the presentation layer (callers
 * gate visibility separately). */
export function selectFailedInboxCount(state: AppState): number {
  return selectActiveWorkspaceState(state).count;
}

/** True only once a successful read has been applied for the active
 * workspace -- neither "never read" nor "the last read failed" counts as
 * known, since AC-UI-INBOX-FAILED-001.23's clause must be omitted in both
 * cases rather than asserting presence or absence. */
export function selectFailedInboxCountIsKnown(state: AppState): boolean {
  return selectActiveWorkspaceState(state).status === "ready";
}
