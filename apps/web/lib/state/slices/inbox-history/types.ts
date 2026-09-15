import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

export type InboxHistoryReadStatus = "idle" | "loading" | "ready" | "error";

export type InboxHistoryWorkspaceState = {
  bundles: InboxHistoryBundle[];
  total: number;
  hasMore: boolean;
  status: InboxHistoryReadStatus;
  // The generation of the last response actually applied to this workspace's
  // rows, for the stale-response guard.
  appliedGeneration: number;
};

export type InboxHistorySliceState = {
  inboxHistory: {
    byWorkspaceId: Record<string, InboxHistoryWorkspaceState>;
    // Latest generation ISSUED per workspace (bumped once per read this
    // client starts). A response is applied only when its generation still
    // equals this counter -- anything older lost the race and is dropped.
    generationByWorkspaceId: Record<string, number>;
  };
};

export type InboxHistorySliceActions = {
  /** Bumps and returns the new request generation for a workspace read. */
  beginInboxHistoryRead: (workspaceId: string) => number;
  /** Applies a successful page read if its generation is still current. */
  setInboxHistoryPage: (
    workspaceId: string,
    generation: number,
    page: { bundles: InboxHistoryBundle[]; total: number; hasMore: boolean },
  ) => void;
  /** Applies a failed read if its generation is still current -- clears rows
   * and total in the same update, so the badge never shows a stale count. */
  setInboxHistoryError: (workspaceId: string, generation: number) => void;
};

export type InboxHistorySlice = InboxHistorySliceState & InboxHistorySliceActions;
