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
    // The workspace `beginFailedInboxRead` was last called for -- lets it
    // tell a genuine workspace switch, which must clear any cached data
    // until the new workspace's response applies, apart from a
    // same-workspace refresh trigger, which must leave already-loaded data
    // alone.
    activeWorkspaceId: string | null;
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
