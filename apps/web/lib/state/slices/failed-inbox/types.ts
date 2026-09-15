import type { FailedInboxRow } from "@/lib/types/failed-inbox";

export type FailedInboxReadStatus = "idle" | "loading" | "ready" | "error";

export type FailedInboxWorkspaceState = {
  rows: FailedInboxRow[];
  count: number;
  truncated: boolean;
  status: FailedInboxReadStatus;
  // The generation of the last response actually applied to this workspace's
  // rows, for the stale-response guard (AC-UI-INBOX-FAILED-001.26).
  appliedGeneration: number;
  // `workspaces.activeIdRevision` as of the read that produced this entry's
  // current status. Compared against the live revision (not a locally-shadowed
  // copy) so a workspace switch that happens while nothing here is mounted to
  // observe it is still detected the next time this workspace is read.
  readAtWorkspaceRevision: number;
};

export type FailedInboxSliceState = {
  failedInbox: {
    byWorkspaceId: Record<string, FailedInboxWorkspaceState>;
    // Latest generation ISSUED per workspace (bumped once per read this
    // client starts). A response is applied only when its generation still
    // equals this counter -- anything older lost the race and is dropped.
    // Keyed on workspace only, deliberately never on the selected tab
    // (design-01#Control-flow).
    generationByWorkspaceId: Record<string, number>;
    // Active-workspace revision captured when each request was issued. A
    // response for an inactive or re-selected workspace must not mutate its
    // cache after the selection boundary has moved.
    readRevisionByWorkspaceId: Record<string, number>;
  };
};

export type FailedInboxSliceActions = {
  /** Bumps and returns the new request generation for a workspace read. */
  beginFailedInboxRead: (workspaceId: string) => number;
  /** Applies a successful page read if its generation is still current. */
  setFailedInboxPage: (
    workspaceId: string,
    generation: number,
    page: { rows: FailedInboxRow[]; count: number; truncated: boolean },
  ) => void;
  /** Applies a failed read if its generation is still current -- clears rows
   * and count in the same update (design-01#Failure-and-recovery). */
  setFailedInboxError: (workspaceId: string, generation: number) => void;
};

export type FailedInboxSlice = FailedInboxSliceState & FailedInboxSliceActions;
