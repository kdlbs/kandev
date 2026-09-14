import type { TFunction } from "i18next";
import { getExecutorLabel } from "@/lib/executor-icons";
import { generateUUID } from "@/lib/utils";
import type {
  ThreadFilterClause,
  ThreadFilterDimension,
  ThreadFilterOp,
  ThreadFilterValue,
  ThreadSortKey,
} from "@/lib/state/slices/ui/thread-view-types";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";

export const MAX_THREAD_VIEW_FILTERS = 20;

export type ThreadDimensionMeta = {
  dimension: ThreadFilterDimension;
  labelKey: string;
  valueKind: "boolean" | "enum" | "text";
  ops: readonly ThreadFilterOp[];
  defaultOp: ThreadFilterOp;
  defaultValue: ThreadFilterValue;
  placeholder?: string;
};

export const THREAD_DIMENSION_METAS: readonly ThreadDimensionMeta[] = [
  {
    dimension: "threadStatus",
    labelKey: "threads:filterThreadStatus",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "needs_action",
  },
  {
    dimension: "pendingAction",
    labelKey: "threads:filterPendingAction",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "clarification",
  },
  {
    dimension: "taskState",
    labelKey: "threads:filterTaskState",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "IN_PROGRESS",
  },
  {
    dimension: "workflow",
    labelKey: "threads:filterWorkflow",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "workflowStep",
    labelKey: "threads:filterWorkflowStep",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "repository",
    labelKey: "threads:filterRepository",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "primaryAgent",
    labelKey: "threads:filterPrimaryAgent",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "executorType",
    labelKey: "threads:filterExecutorType",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "priority",
    labelKey: "threads:filterPriority",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "medium",
  },
  {
    dimension: "blocked",
    labelKey: "threads:filterBlocked",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasQueuedPrompts",
    labelKey: "threads:filterQueuedPrompts",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasActiveSubagents",
    labelKey: "threads:filterActiveSubagents",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasDiff",
    labelKey: "threads:filterDiff",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasPR",
    labelKey: "threads:filterPullRequest",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "prNeedsAttention",
    labelKey: "threads:filterPullRequestAttention",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "taskType",
    labelKey: "threads:filterTaskType",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "standard",
  },
  {
    dimension: "titleMatch",
    labelKey: "threads:filterTitle",
    valueKind: "text",
    ops: ["matches", "not_matches"],
    defaultOp: "matches",
    defaultValue: "",
  },
  {
    dimension: "hasActiveError",
    labelKey: "threads:filterActiveError",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "taskLabel",
    labelKey: "threads:filterTaskLabel",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "taskOrigin",
    labelKey: "threads:filterTaskOrigin",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "manual",
  },
  {
    dimension: "hasMultipleSessions",
    labelKey: "threads:filterMultipleSessions",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
];

export const THREAD_SORT_OPTIONS: readonly {
  key: ThreadSortKey;
  labelKey: string;
  descriptionKey: string;
}[] = [
  {
    key: "attention",
    labelKey: "threads:sortAttention",
    descriptionKey: "threads:sortAttentionDescription",
  },
  {
    key: "lastActivityAt",
    labelKey: "threads:sortLastActivity",
    descriptionKey: "threads:sortLastActivityDescription",
  },
  {
    key: "updatedAt",
    labelKey: "threads:sortUpdated",
    descriptionKey: "threads:sortUpdatedDescription",
  },
  {
    key: "createdAt",
    labelKey: "threads:sortCreated",
    descriptionKey: "threads:sortCreatedDescription",
  },
  {
    key: "title",
    labelKey: "threads:sortTitle",
    descriptionKey: "threads:sortTitleDescription",
  },
  {
    key: "taskState",
    labelKey: "threads:sortTaskState",
    descriptionKey: "threads:sortTaskStateDescription",
  },
  {
    key: "workflow",
    labelKey: "threads:sortWorkflow",
    descriptionKey: "threads:sortWorkflowDescription",
  },
  {
    key: "priority",
    labelKey: "threads:sortPriority",
    descriptionKey: "threads:sortPriorityDescription",
  },
  {
    key: "primaryAgent",
    labelKey: "threads:sortPrimaryAgent",
    descriptionKey: "threads:sortPrimaryAgentDescription",
  },
];

const FIXED_OPTIONS: Partial<
  Record<ThreadFilterDimension, readonly { value: string; labelKey: string }[]>
> = {
  threadStatus: [
    { value: "needs_action", labelKey: "threads:statusNeedsAction" },
    { value: "running", labelKey: "threads:statusRunning" },
    { value: "waiting", labelKey: "threads:statusWaiting" },
    { value: "ready_for_review", labelKey: "threads:statusReadyForReview" },
  ],
  pendingAction: [
    { value: "clarification", labelKey: "threads:pendingClarification" },
    { value: "permission", labelKey: "threads:pendingPermission" },
    { value: "none", labelKey: "threads:pendingNone" },
  ],
  taskType: [
    { value: "standard", labelKey: "threads:taskTypeStandard" },
    { value: "pull_request_review", labelKey: "threads:taskTypePullRequestReview" },
    { value: "issue_watch", labelKey: "threads:taskTypeIssueWatch" },
  ],
};

const TASK_STATE_LABEL_KEYS: Record<string, string> = {
  CREATED: "threads:statusNotStarted",
  SCHEDULING: "threads:statusStarting",
  TODO: "task:statusTodo",
  IN_PROGRESS: "task:statusInProgress",
  WAITING_FOR_INPUT: "threads:statusWaiting",
  BLOCKED: "task:statusBlocked",
  REVIEW: "task:statusInReview",
  COMPLETED: "task:statusCompleted",
  FAILED: "threads:statusFailed",
  CANCELLED: "task:statusCancelled",
};

const PRIORITY_LABEL_KEYS: Record<string, string> = {
  critical: "task:priorityCritical",
  high: "task:priorityHigh",
  medium: "task:priorityMedium",
  low: "task:priorityLow",
};

const TASK_ORIGIN_LABEL_KEYS: Record<string, string> = {
  manual: "threads:originManual",
  agent_created: "threads:originAgentCreated",
  routine: "threads:originRoutine",
  onboarding: "threads:originOnboarding",
  automation_run: "threads:originAutomationRun",
  automation_task: "threads:originAutomationTask",
};

export function createThreadFilterClause(): ThreadFilterClause {
  const meta = THREAD_DIMENSION_METAS[0];
  return {
    id: `thread-filter-${generateUUID()}`,
    dimension: meta.dimension,
    op: meta.defaultOp,
    value: meta.defaultValue,
  };
}

export function getThreadDimensionMeta(dimension: ThreadFilterDimension): ThreadDimensionMeta {
  return (
    THREAD_DIMENSION_METAS.find((meta) => meta.dimension === dimension) ?? THREAD_DIMENSION_METAS[0]
  );
}

export function getThreadDimensionLabel(dimension: ThreadFilterDimension, t: TFunction): string {
  return t(getThreadDimensionMeta(dimension).labelKey);
}

export function getThreadFilterOptions(
  dimension: ThreadFilterDimension,
  candidates: ThreadCandidate[],
  t: TFunction,
  repositoryNames: ReadonlyMap<string, string> = new Map(),
): Array<{ value: string; label: string; group?: string }> {
  const fixed = FIXED_OPTIONS[dimension];
  if (fixed) return fixed.map((option) => ({ value: option.value, label: t(option.labelKey) }));

  const values = new Map<string, { label: string; group?: string }>();
  const addOption = CANDIDATE_OPTION_BUILDERS[dimension];
  for (const candidate of candidates) addOption?.(candidate, values);
  return [...values.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([value, option]) => ({
      value,
      ...option,
      label: getThreadFilterOptionLabel(dimension, value, option.label, t, repositoryNames),
    }));
}

function getThreadFilterOptionLabel(
  dimension: ThreadFilterDimension,
  value: string,
  fallback: string,
  t: TFunction,
  repositoryNames: ReadonlyMap<string, string>,
): string {
  if (dimension === "repository") return repositoryNames.get(value) ?? fallback;
  if (dimension === "executorType") return getExecutorLabel(value);
  if (dimension === "taskState")
    return TASK_STATE_LABEL_KEYS[value] ? t(TASK_STATE_LABEL_KEYS[value]) : value;
  if (dimension === "priority")
    return PRIORITY_LABEL_KEYS[value] ? t(PRIORITY_LABEL_KEYS[value]) : value;
  if (dimension === "taskOrigin")
    return TASK_ORIGIN_LABEL_KEYS[value] ? t(TASK_ORIGIN_LABEL_KEYS[value]) : value;
  return fallback;
}

type CandidateOptionAccumulator = Map<string, { label: string; group?: string }>;
type CandidateOptionBuilder = (
  candidate: ThreadCandidate,
  values: CandidateOptionAccumulator,
) => void;

const CANDIDATE_OPTION_BUILDERS: Partial<Record<ThreadFilterDimension, CandidateOptionBuilder>> = {
  workflow: (candidate, values) =>
    values.set(candidate.workflowId, { label: candidate.workflowName }),
  workflowStep: (candidate, values) =>
    values.set(candidate.workflowStepId, {
      label: candidate.stepTitle ?? candidate.workflowStepId,
      group: candidate.workflowName,
    }),
  repository: (candidate, values) => {
    for (const value of candidate.repositoryIds) values.set(value, { label: value });
  },
  primaryAgent: (candidate, values) => {
    if (candidate.primaryAgentProfileId) {
      values.set(candidate.primaryAgentProfileId, {
        label: candidate.primaryAgentName ?? candidate.primaryAgentProfileId,
      });
    }
  },
  executorType: (candidate, values) => {
    if (candidate.executorType)
      values.set(candidate.executorType, { label: candidate.executorType });
  },
  priority: (candidate, values) => {
    if (candidate.priority) values.set(candidate.priority, { label: candidate.priority });
  },
  taskState: (candidate, values) => {
    if (candidate.taskState) values.set(candidate.taskState, { label: candidate.taskState });
  },
  taskLabel: (candidate, values) => {
    for (const value of candidate.labels) values.set(value, { label: value });
  },
  taskOrigin: (candidate, values) =>
    values.set(candidate.taskOrigin, { label: candidate.taskOrigin }),
};

export function normaliseThreadClauseValue(
  value: ThreadFilterValue,
  meta: ThreadDimensionMeta,
  op: ThreadFilterOp,
): ThreadFilterValue {
  if (meta.valueKind === "boolean") return true;
  if (op === "in" || op === "not_in") {
    if (Array.isArray(value)) return value;
    return value ? [String(value)] : [];
  }
  return Array.isArray(value) ? (value[0] ?? "") : value;
}

export function getThreadFilterOpLabel(op: ThreadFilterOp, t: TFunction): string {
  if (op === "is") return t("threads:filterIs");
  if (op === "is_not") return t("threads:filterIsNot");
  if (op === "in") return t("threads:filterIn");
  if (op === "not_in") return t("threads:filterNotIn");
  if (op === "matches") return t("threads:filterMatches");
  return t("threads:filterNotMatches");
}
