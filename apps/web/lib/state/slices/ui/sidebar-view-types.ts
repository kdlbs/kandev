export type FilterDimension =
  | "archived"
  | "state"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "repository"
  | "hasDiff"
  | "isPRReview"
  | "isIssueWatch"
  | "titleMatch";

export type FilterOp = "is" | "is_not" | "in" | "not_in" | "matches" | "not_matches";

export type FilterValue = string | string[] | boolean;

export type FilterClause = {
  id: string;
  dimension: FilterDimension;
  op: FilterOp;
  value: FilterValue;
};

export type SortKey = "state" | "updatedAt" | "createdAt" | "title";
export type SortDirection = "asc" | "desc";
export type SortSpec = { key: SortKey; direction: SortDirection };

export type GroupKey =
  | "none"
  | "repository"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "state";

export type SidebarView = {
  id: string;
  name: string;
  filters: FilterClause[];
  sort: SortSpec;
  group: GroupKey;
  collapsedGroups: string[];
};

export type SidebarSliceState = {
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
  /** Last error surfaced by an async backend sync. Consumed by a toast bridge. */
  syncError: string | null;
};

export type SidebarViewDraft = {
  baseViewId: string;
  filters: FilterClause[];
  sort: SortSpec;
  group: GroupKey;
};
