export type ThreadTaskScope =
  | { mode: "all"; taskIds: [] }
  | { mode: "selected"; taskIds: string[] };

export type ThreadFilterDimension =
  | "threadStatus"
  | "pendingAction"
  | "taskState"
  | "workflow"
  | "workflowStep"
  | "repository"
  | "primaryAgent"
  | "executorType"
  | "priority"
  | "blocked"
  | "hasQueuedPrompts"
  | "hasActiveSubagents"
  | "hasDiff"
  | "hasPR"
  | "prNeedsAttention"
  | "taskType"
  | "titleMatch"
  | "hasActiveError"
  | "taskLabel"
  | "taskOrigin"
  | "hasMultipleSessions";

export type ThreadFilterOp = "is" | "is_not" | "in" | "not_in" | "matches" | "not_matches";
export type ThreadFilterValue = string | string[] | boolean;

export type ThreadFilterClause = {
  id: string;
  dimension: ThreadFilterDimension;
  op: ThreadFilterOp;
  value: ThreadFilterValue;
};

export type ThreadSortKey =
  | "attention"
  | "lastActivityAt"
  | "updatedAt"
  | "createdAt"
  | "title"
  | "taskState"
  | "workflow"
  | "priority"
  | "primaryAgent";
export type ThreadSortDirection = "asc" | "desc";
export type ThreadSortSpec = { key: ThreadSortKey; direction: ThreadSortDirection };

export type ThreadView = {
  id: string;
  name: string;
  taskScope: ThreadTaskScope;
  filters: ThreadFilterClause[];
  sort: ThreadSortSpec;
  maxColumns: number | null;
};

export type ThreadViewDraft = Omit<ThreadView, "id" | "name"> & {
  baseViewId: string;
};

/** A complete backend-owned Threads view projection kept during a local write. */
export type ThreadViewSnapshot = {
  views: ThreadView[];
  activeViewId: string;
  draft: ThreadViewDraft | null;
};

export type ThreadViewSliceState = {
  views: ThreadView[];
  activeViewId: string;
  draft: ThreadViewDraft | null;
  syncError: string | null;
  /** Keep server echoes from replacing optimistic edits until the latest write settles. */
  syncPending: boolean;
  /** Newer authoritative settings received while an optimistic write is pending. */
  deferredServerState: ThreadViewSnapshot | null;
  orderResetGeneration: number;
};
