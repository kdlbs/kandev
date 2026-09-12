import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";

export type NeedsYouInboxReadStatus = "idle" | "loading" | "ready" | "error";

export type NeedsYouInboxWorkspaceState = {
  bundles: ClarificationInboxBundle[];
  count: number;
  hiddenCount: number;
  nextSnoozeExpiry: string | null;
  hasMore: boolean;
  status: NeedsYouInboxReadStatus;
  // The generation of the last response actually applied to this workspace's
  // rows, for the AC .38 stale-response guard.
  appliedGeneration: number;
};

export type NeedsYouInboxSliceState = {
  needsYouInbox: {
    byWorkspaceId: Record<string, NeedsYouInboxWorkspaceState>;
    // Latest generation ISSUED per workspace (bumped once per read this
    // client starts). A response is applied only when its generation still
    // equals this counter -- anything older lost the race and is dropped.
    generationByWorkspaceId: Record<string, number>;
    // Bumped by the WS-action refresh triggers (design-02#Control-flow):
    // session.pending_action_changed and session.state_changed. Not
    // per-workspace -- every trigger re-reads whichever workspace is active
    // when it fires, never the one the event happened to name.
    refreshTick: number;
  };
};

export type NeedsYouInboxSliceActions = {
  /** Bumps and returns the new request generation for a workspace read. */
  beginNeedsYouInboxRead: (workspaceId: string) => number;
  /** Applies a successful page read if its generation is still current. */
  setNeedsYouInboxPage: (
    workspaceId: string,
    generation: number,
    page: {
      bundles: ClarificationInboxBundle[];
      count: number;
      hiddenCount: number;
      nextSnoozeExpiry: string | null;
      hasMore: boolean;
    },
  ) => void;
  /** Applies a failed read if its generation is still current -- clears rows
   * and count in the same update (design-02#Failure-and-recovery). */
  setNeedsYouInboxError: (workspaceId: string, generation: number) => void;
  /** Seeds a workspace from boot hydration at generation 0, so the first real
   * read (generation >= 1) always supersedes it. */
  seedNeedsYouInboxBoot: (
    workspaceId: string,
    seed: { count: number; hasMore: boolean; nextSnoozeExpiry: string | null },
  ) => void;
  /** Bumps the WS-action refresh trigger tick (design-02#Control-flow). */
  bumpNeedsYouInboxRefreshTick: () => void;
};

export type NeedsYouInboxSlice = NeedsYouInboxSliceState & NeedsYouInboxSliceActions;
