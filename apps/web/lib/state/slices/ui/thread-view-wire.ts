import type {
  ThreadTaskScopeApi,
  ThreadViewApi,
  ThreadViewClauseApi,
  ThreadViewDraftApi,
} from "@/lib/types/http-user-settings";
import type {
  ThreadFilterClause,
  ThreadFilterDimension,
  ThreadFilterOp,
  ThreadFilterValue,
  ThreadSortDirection,
  ThreadSortKey,
  ThreadTaskScope,
  ThreadView,
  ThreadViewDraft,
} from "./thread-view-types";

export const THREAD_FILTER_DIMENSIONS: readonly ThreadFilterDimension[] = [
  "threadStatus",
  "pendingAction",
  "taskState",
  "workflow",
  "workflowStep",
  "repository",
  "primaryAgent",
  "executorType",
  "priority",
  "blocked",
  "hasQueuedPrompts",
  "hasActiveSubagents",
  "hasDiff",
  "hasPR",
  "prNeedsAttention",
  "taskType",
  "titleMatch",
  "hasActiveError",
  "taskLabel",
  "taskOrigin",
  "hasMultipleSessions",
];

export const THREAD_SORT_KEYS: readonly ThreadSortKey[] = [
  "attention",
  "lastActivityAt",
  "updatedAt",
  "createdAt",
  "title",
  "taskState",
  "workflow",
  "priority",
  "primaryAgent",
];

const THREAD_FILTER_OPS: readonly ThreadFilterOp[] = [
  "is",
  "is_not",
  "in",
  "not_in",
  "matches",
  "not_matches",
];

const MAX_THREAD_TASK_IDS = 200;
const MAX_THREAD_FILTERS = 20;

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isOneOf<T extends string>(value: unknown, values: readonly T[]): value is T {
  return isString(value) && values.includes(value as T);
}

function normalizeTaskIds(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  return value
    .filter(isString)
    .map((taskId) => taskId.trim())
    .filter((taskId) => taskId.length > 0 && !seen.has(taskId) && seen.add(taskId))
    .slice(0, MAX_THREAD_TASK_IDS);
}

export function fromApiThreadTaskScope(
  api: ThreadTaskScopeApi | null | undefined,
): ThreadTaskScope {
  const taskIds = normalizeTaskIds(api?.task_ids);
  if (api?.mode === "selected") return { mode: "selected", taskIds };
  return { mode: "all", taskIds: [] };
}

function normalizeFilterValue(value: unknown): ThreadFilterValue | null {
  if (typeof value === "boolean" || typeof value === "string") return value;
  if (!Array.isArray(value) || !value.every(isString)) return null;
  return value.map((item) => item.trim()).filter(Boolean);
}

function fromApiClause(api: ThreadViewClauseApi): ThreadFilterClause | null {
  if (!isString(api?.id) || api.id.trim() === "") return null;
  if (!isOneOf(api.dimension, THREAD_FILTER_DIMENSIONS)) return null;
  if (!isOneOf(api.op, THREAD_FILTER_OPS)) return null;
  const value = normalizeFilterValue(api.value);
  if (value === null) return null;
  return { id: api.id, dimension: api.dimension, op: api.op, value };
}

function fromApiFilters(value: unknown): ThreadFilterClause[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((clause) =>
      clause && typeof clause === "object" ? fromApiClause(clause as ThreadViewClauseApi) : null,
    )
    .filter((clause): clause is ThreadFilterClause => clause !== null)
    .slice(0, MAX_THREAD_FILTERS);
}

function normalizeSort(value: ThreadViewApi["sort"] | null | undefined) {
  const key = isOneOf(value?.key, THREAD_SORT_KEYS) ? value.key : "attention";
  const direction: ThreadSortDirection = value?.direction === "desc" ? "desc" : "asc";
  return { key, direction };
}

function normalizeMaxColumns(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= 30
    ? value
    : null;
}

export function fromApiThreadView(api: ThreadViewApi): ThreadView {
  return {
    id: isString(api?.id) ? api.id : "",
    name: isString(api?.name) ? api.name : "",
    taskScope: fromApiThreadTaskScope(api?.task_scope),
    filters: fromApiFilters(api?.filters),
    sort: normalizeSort(api?.sort),
    maxColumns: normalizeMaxColumns(api?.max_columns),
  };
}

export function fromApiThreadDraft(api: ThreadViewDraftApi): ThreadViewDraft {
  return {
    baseViewId: isString(api?.base_view_id) ? api.base_view_id : "",
    taskScope: fromApiThreadTaskScope(api?.task_scope),
    filters: fromApiFilters(api?.filters),
    sort: normalizeSort(api?.sort),
    maxColumns: normalizeMaxColumns(api?.max_columns),
  };
}

function toApiTaskScope(scope: ThreadTaskScope) {
  return { mode: scope.mode, task_ids: [...scope.taskIds] };
}

function toApiClause(clause: ThreadFilterClause) {
  return {
    id: clause.id,
    dimension: clause.dimension,
    op: clause.op,
    value: clause.value,
  };
}

export function toApiThreadView(view: ThreadView): ThreadViewApi {
  return {
    id: view.id,
    name: view.name,
    task_scope: toApiTaskScope(view.taskScope),
    filters: view.filters.map(toApiClause),
    sort: { key: view.sort.key, direction: view.sort.direction },
    max_columns: view.maxColumns,
  };
}

export function toApiThreadDraft(draft: ThreadViewDraft): ThreadViewDraftApi {
  return {
    base_view_id: draft.baseViewId,
    task_scope: toApiTaskScope(draft.taskScope),
    filters: draft.filters.map(toApiClause),
    sort: { key: draft.sort.key, direction: draft.sort.direction },
    max_columns: draft.maxColumns,
  };
}
