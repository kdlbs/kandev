import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

export type InboxHistoryReadStatus = "idle" | "loading" | "ready" | "error";

export type InboxHistoryWorkspaceState = {
  bundles: InboxHistoryBundle[];
  total: number;
  hasMore: boolean;
  nextCursor?: string;
  isLoadingMore: boolean;
  loadMoreError: boolean;
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
  /** Starts a cursor read if the workspace is ready and has another page. */
  beginInboxHistoryLoadMore: (workspaceId: string, generation: number) => boolean;
  /** Applies a successful page read if its generation is still current. */
  setInboxHistoryPage: (
    workspaceId: string,
    generation: number,
    page: {
      bundles: InboxHistoryBundle[];
      total: number;
      hasMore: boolean;
      nextCursor?: string;
    },
  ) => void;
  /** Appends a cursor page if its generation is still current. */
  appendInboxHistoryPage: (
    workspaceId: string,
    generation: number,
    page: {
      bundles: InboxHistoryBundle[];
      total: number;
      hasMore: boolean;
      nextCursor?: string;
    },
  ) => void;
  /** Leaves existing rows visible when a cursor read fails. */
  setInboxHistoryLoadMoreError: (workspaceId: string, generation: number) => void;
  /** Applies a failed read if its generation is still current -- clears rows
   * and total in the same update, so the badge never shows a stale count. */
  setInboxHistoryError: (workspaceId: string, generation: number) => void;
};

export type InboxHistorySlice = InboxHistorySliceState & InboxHistorySliceActions;
