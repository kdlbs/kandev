import type { SidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

export type FilterDimension =
  | "archived"
  | "state"
  | "workflow"
  | "workflowStep"
  | "executorType"
  | "repository"
  | "hasDiff"
  | "hasPR"
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

export type SortKey =
  | "state"
  | "updatedAt"
  | "lastActivityAt"
  | "createdAt"
  | "title"
  | "running"
  | "color"
  | "custom";
export type SortDirection = "asc" | "desc";
export type SidebarColorToken =
  | "gray"
  | "red"
  | "orange"
  | "yellow"
  | "green"
  | "cyan"
  | "blue"
  | "indigo"
  | "purple"
  | "pink";
export type SortCriterion = {
  key: Exclude<SortKey, "custom">;
  direction: SortDirection;
  color?: SidebarColorToken;
};
export type SortRule = SortCriterion | { key: "custom"; direction: SortDirection };
export type SortSpec = {
  key: SortKey;
  direction: SortDirection;
  color?: SidebarColorToken;
  thenBy?: SortCriterion[];
};

export type { SidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

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
  sortWarningCount?: number;
  group: GroupKey;
  groupIndent: boolean;
  collapsedGroups: string[];
  taskRow?: SidebarTaskRowPresentation;
};

export type SidebarSliceState = {
  syncPending?: boolean;
  serverRevision?: number | null;
  deferredServerState?: SidebarSliceState | null;
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
  sortWarningCount?: number;
  group: GroupKey;
  groupIndent: boolean;
  taskRow?: SidebarTaskRowPresentation;
};
