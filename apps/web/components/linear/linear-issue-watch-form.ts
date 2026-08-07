import type {
  LinearIssueSortBy,
  LinearIssueWatch,
  LinearSearchFilter,
  LinearUser,
} from "@/lib/types/linear";
import { DEFAULT_LINEAR_ISSUE_WATCH_PROMPT } from "./linear-issue-watch-placeholders";

export const ASSIGNED_ANY = "__any__";
export const CREATOR_ANY = "__any__";

export type LinearPriority = 0 | 1 | 2 | 3 | 4;

// Linear priorities: 0=None, 1=Urgent, 2=High, 3=Medium, 4=Low. Rendered as
// toggle chips, mirroring the States and Labels multi-selects.
//
// `value` is a numeric union sent to the backend as the filter's priority list
// — never translated. The chip text travels as `labelKey` and resolves at
// render through `resolveOptionLabel`; a module-scope `t()` here would freeze
// the copy at the boot locale (see docs/i18n.md).
export const PRIORITY_OPTIONS: { value: LinearPriority; labelKey: string }[] = [
  { value: 1, labelKey: "linear:priorityUrgent" },
  { value: 2, labelKey: "linear:priorityHigh" },
  { value: 3, labelKey: "linear:priorityMedium" },
  { value: 4, labelKey: "linear:priorityLow" },
  { value: 0, labelKey: "linear:priorityNone" },
];

// Dispatch order applied when the in-flight cap limits how many matched issues
// run at once. Order matters — most useful first; the empty value is Linear's
// natural (recently-updated) order.
//
// `value` is a `LinearIssueSortBy` string-literal union: persisted on the
// watch, compared against `form.sortBy`, and sent upstream. Only `labelKey` is
// copy.
export const SORT_BY_OPTIONS: { value: LinearIssueSortBy; labelKey: string }[] = [
  { value: "priority", labelKey: "linear:sortPriorityDesc" },
  { value: "priority_asc", labelKey: "linear:sortPriorityAsc" },
  { value: "created_desc", labelKey: "linear:sortCreatedDesc" },
  { value: "created_asc", labelKey: "linear:sortCreatedAsc" },
  { value: "updated_desc", labelKey: "linear:sortUpdatedDesc" },
  { value: "updated_asc", labelKey: "linear:sortUpdatedAsc" },
  { value: "", labelKey: "linear:sortDefault" },
];

export interface FormState {
  workspaceId: string;
  query: string;
  teamKey: string;
  stateIds: string[];
  assigned: string;
  priorities: LinearPriority[];
  labelIds: string[];
  creatorId: string;
  estimateMin: string;
  estimateMax: string;
  workflowId: string;
  workflowStepId: string;
  /** Optional repository binding; "" = unbound (repo-less task). */
  repositoryId: string;
  /** Base branch for the worktree; "" = the repository's default branch. */
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollInterval: number;
  /**
   * Per-watcher throttle cap as a free-text input: empty string means
   * "uncapped" (sent as null), non-empty must parse to a positive integer.
   * Kept as a string so the user can clear the field without it snapping
   * back to a number.
   */
  maxInflightTasks: string;
  /** Dispatch order under the in-flight cap; empty = Linear's natural order. */
  sortBy: LinearIssueSortBy;
}

export function makeEmptyForm(workspaceId: string): FormState {
  return {
    workspaceId,
    query: "",
    teamKey: "",
    stateIds: [],
    assigned: "",
    priorities: [],
    labelIds: [],
    creatorId: "",
    estimateMin: "",
    estimateMax: "",
    workflowId: "",
    workflowStepId: "",
    repositoryId: "",
    baseBranch: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: true,
    pollInterval: 300,
    maxInflightTasks: "5",
    sortBy: "priority",
  };
}

function estimateString(v: number | null | undefined): string {
  return v === undefined || v === null ? "" : String(v);
}

export function formStateFromWatch(w: LinearIssueWatch): FormState {
  const f: LinearSearchFilter = w.filter ?? {};
  return {
    workspaceId: w.workspaceId,
    query: f.query ?? "",
    teamKey: f.teamKey ?? "",
    stateIds: f.stateIds ?? [],
    assigned: f.assigned ?? "",
    priorities: f.priorities ?? [],
    labelIds: f.labelIds ?? [],
    creatorId: f.creatorId ?? "",
    estimateMin: estimateString(f.estimateMin),
    estimateMax: estimateString(f.estimateMax),
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    repositoryId: w.repositoryId ?? "",
    baseBranch: w.baseBranch ?? "",
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt || DEFAULT_LINEAR_ISSUE_WATCH_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
    maxInflightTasks: maxInflightTasksString(w.maxInflightTasks),
    sortBy: w.sortBy ?? "",
  };
}

/**
 * Renders the watch's stored throttle cap for the dialog input. `null` /
 * `undefined` map to an empty string ("uncapped"); positive integers are
 * shown as-is. Non-positive values from a stale row are clamped to empty
 * — the backend rejects them on save anyway, and showing a misleading "0"
 * would let users think the cap was enforced.
 */
export function maxInflightTasksString(v: number | null | undefined): string {
  if (v === undefined || v === null) return "";
  if (!Number.isFinite(v) || v <= 0) return "";
  return String(v);
}

/**
 * Parses the throttle-cap input back into a payload value. Returns:
 *  - `null` when the input is blank (user wants uncapped),
 *  - the integer when it's a positive whole number,
 *  - `"invalid"` when the input is non-empty but unparseable / non-positive,
 *    so the dialog can surface an inline validation error before submit.
 */
export function parseMaxInflightTasks(raw: string): number | null | "invalid" {
  const t = raw.trim();
  if (t === "") return null;
  const n = Number(t);
  if (!Number.isInteger(n) || n <= 0) return "invalid";
  return n;
}

/**
 * Aggregates the dialog's "can the Save button enable?" rule. Pulled out of
 * the dialog so the file stays under the 600-line ceiling and the rule has
 * one named home — handy for unit tests as the form grows.
 */
export function isWatchFormReady(form: FormState): boolean {
  return (
    !!form.workspaceId &&
    !filterIsEmpty(form) &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    !!form.prompt.trim() &&
    parseMaxInflightTasks(form.maxInflightTasks) !== "invalid"
  );
}

/**
 * Builds the API payload from the dialog state. Returns `null` when the
 * throttle cap input fails validation, so the caller can short-circuit
 * the submit without throwing.
 */
export function buildWatchPayload(form: FormState): {
  filter: LinearSearchFilter;
  workflowId: string;
  workflowStepId: string;
  repositoryId: string;
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  maxInflightTasks: number | null;
  sortBy: LinearIssueSortBy;
} | null {
  const maxInflight = parseMaxInflightTasks(form.maxInflightTasks);
  if (maxInflight === "invalid") return null;
  return {
    filter: buildFilterPayload(form),
    workflowId: form.workflowId,
    workflowStepId: form.workflowStepId,
    // An empty repositoryId clears the binding; the empty base branch is sent
    // verbatim so the backend fills it with the repo's default at save time.
    repositoryId: form.repositoryId,
    baseBranch: form.repositoryId ? form.baseBranch : "",
    agentProfileId: form.agentProfileId,
    executorProfileId: form.executorProfileId,
    prompt: form.prompt,
    enabled: form.enabled,
    pollIntervalSeconds: form.pollInterval,
    maxInflightTasks: maxInflight,
    sortBy: form.sortBy,
  };
}

export function parseEstimate(raw: string): number | undefined {
  const t = raw.trim();
  if (t === "") return undefined;
  const n = Number(t);
  if (!Number.isFinite(n) || n < 0) return undefined;
  return n;
}

export function filterIsEmpty(form: FormState): boolean {
  return (
    form.query.trim() === "" &&
    form.teamKey.trim() === "" &&
    form.assigned.trim() === "" &&
    form.stateIds.length === 0 &&
    form.priorities.length === 0 &&
    form.labelIds.length === 0 &&
    form.creatorId.trim() === "" &&
    parseEstimate(form.estimateMin) === undefined &&
    parseEstimate(form.estimateMax) === undefined
  );
}

export function buildFilterPayload(form: FormState): LinearSearchFilter {
  return {
    query: form.query.trim() || undefined,
    teamKey: form.teamKey.trim() || undefined,
    stateIds: form.stateIds.length > 0 ? form.stateIds : undefined,
    assigned: form.assigned.trim() || undefined,
    priorities: form.priorities.length > 0 ? form.priorities : undefined,
    labelIds: form.labelIds.length > 0 ? form.labelIds : undefined,
    creatorId: form.creatorId.trim() || undefined,
    estimateMin: parseEstimate(form.estimateMin),
    estimateMax: parseEstimate(form.estimateMax),
  };
}

/**
 * Display text for one Linear user in the Creator picker. Every branch is user
 * data straight from the Linear API — a display name, a full name, an email
 * address, or the raw id — so nothing here is translatable copy.
 */
export function userOptionLabel(u: LinearUser): string {
  const name = u.displayName?.trim() || u.name?.trim() || u.email?.trim() || u.id;
  if (u.email && u.email !== name) return `${name} (${u.email})`;
  return name;
}

// `t` is threaded in rather than read from a hook: this is a plain function in a
// `.ts` module, and `mode: "jsx-only"` means the guard never inspects this file,
// so a literal here would survive lint. The caller passes its `useTranslation`
// `t`, which keeps the copy resolving at render.
export function creatorPlaceholder(
  t: (key: string) => string,
  teamKey: string,
  loadingUsers: boolean,
): string {
  if (loadingUsers) return t("linear:loadingEllipsis");
  if (!teamKey) return t("linear:pickATeamFirst");
  return t("linear:any");
}
