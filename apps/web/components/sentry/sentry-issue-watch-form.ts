import type {
  SentryIssueWatch,
  SentryLevel,
  SentryProject,
  SentrySearchFilter,
  SentryStatus,
} from "@/lib/types/sentry";
import { DEFAULT_SENTRY_ISSUE_WATCH_PROMPT } from "./sentry-issue-watch-placeholders";

// Every `value` below is wire data, never copy: the levels and statuses are the
// `SentryLevel` / `SentryStatus` string-literal unions — compared with
// `includes`, persisted on the watch filter, and sent to Sentry verbatim — and
// the stats periods are Sentry's own `statsPeriod` tokens. Only the chip and
// menu text is translatable, so it travels as `labelKey` and resolves at render
// through `resolveOptionLabel`; a module-scope `t()` here would freeze the copy
// at the boot locale (see docs/i18n.md).
//
// The level/status labels are lowercase in the catalog because the chips render
// them under a CSS `uppercase`, so the source string a translator sees is the
// one the badge actually carries.
export const LEVEL_OPTIONS: { value: SentryLevel; labelKey: string }[] = [
  { value: "fatal", labelKey: "sentry:levelFatal" },
  { value: "error", labelKey: "sentry:levelError" },
  { value: "warning", labelKey: "sentry:levelWarning" },
  { value: "info", labelKey: "sentry:levelInfo" },
  { value: "debug", labelKey: "sentry:levelDebug" },
];

export const STATUS_OPTIONS: { value: SentryStatus; labelKey: string }[] = [
  { value: "unresolved", labelKey: "sentry:statusUnresolved" },
  { value: "resolved", labelKey: "sentry:statusResolved" },
  { value: "ignored", labelKey: "sentry:statusIgnored" },
];

export const STATS_PERIOD_OPTIONS: { value: string; labelKey: string }[] = [
  { value: "1h", labelKey: "sentry:statsPeriodLastHour" },
  { value: "24h", labelKey: "sentry:statsPeriodLast24Hours" },
  { value: "7d", labelKey: "sentry:statsPeriodLast7Days" },
  { value: "14d", labelKey: "sentry:statsPeriodLast14Days" },
  { value: "30d", labelKey: "sentry:statsPeriodLast30Days" },
];

export interface FormState {
  workspaceId: string;
  /** The Sentry instance to poll. Required; immutable after create. */
  sentryInstanceId: string;
  orgSlug: string;
  projectSlugs: string[];
  environment: string;
  levels: SentryLevel[];
  statuses: SentryStatus[];
  query: string;
  statsPeriod: string;
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
  maxInflightTasks: string;
}

export function makeEmptyForm(workspaceId: string): FormState {
  return {
    workspaceId,
    sentryInstanceId: "",
    orgSlug: "",
    projectSlugs: [],
    environment: "",
    levels: ["fatal", "error"],
    statuses: ["unresolved"],
    query: "",
    statsPeriod: "24h",
    workflowId: "",
    workflowStepId: "",
    repositoryId: "",
    baseBranch: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_SENTRY_ISSUE_WATCH_PROMPT,
    enabled: true,
    pollInterval: 300,
    maxInflightTasks: "5",
  };
}

export function formStateFromWatch(w: SentryIssueWatch): FormState {
  const f: SentrySearchFilter = w.filter ?? { orgSlug: "" };
  return {
    workspaceId: w.workspaceId,
    sentryInstanceId: w.sentryInstanceId ?? "",
    orgSlug: f.orgSlug ?? "",
    projectSlugs: f.projectSlugs ?? [],
    environment: f.environment ?? "",
    levels: f.levels ?? [],
    statuses: f.statuses ?? [],
    query: f.query ?? "",
    statsPeriod: f.statsPeriod ?? "",
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
    repositoryId: w.repositoryId ?? "",
    baseBranch: w.baseBranch ?? "",
    agentProfileId: w.agentProfileId,
    executorProfileId: w.executorProfileId,
    prompt: w.prompt || DEFAULT_SENTRY_ISSUE_WATCH_PROMPT,
    enabled: w.enabled,
    pollInterval: w.pollIntervalSeconds,
    maxInflightTasks: maxInflightTasksString(w.maxInflightTasks),
  };
}

/**
 * Formats the throttle cap for the input. nil/undefined and non-positive
 * (from a stale row) collapse to "" — an empty box reads as "no cap", and
 * showing "0" would falsely imply a cap was enforced.
 */
export function maxInflightTasksString(v: number | null | undefined): string {
  if (v === undefined || v === null) return "";
  if (!Number.isFinite(v) || v <= 0) return "";
  return String(v);
}

/**
 * Parses the throttle-cap input back into a payload value. Returns `null` for
 * blank (uncapped), the integer for a positive whole number, or "invalid" when
 * the input is non-empty but unparseable / non-positive so the dialog can show
 * an inline error before submit.
 */
export function parseMaxInflightTasks(raw: string): number | null | "invalid" {
  const t = raw.trim();
  if (t === "") return null;
  const n = Number(t);
  if (!Number.isInteger(n) || n <= 0) return "invalid";
  return n;
}

export type SelectItemSpec = { id: string; label: string };

export function orgSelectItems(orgs: string[], current: string): SelectItemSpec[] {
  const items: SelectItemSpec[] = [];
  const seen = new Set<string>();
  // Include the current value even if the token can no longer see it (editing an
  // old watch) so the Select still shows the saved org.
  for (const slug of [current, ...orgs]) {
    if (!slug || seen.has(slug)) continue;
    seen.add(slug);
    items.push({ id: slug, label: slug });
  }
  return items;
}

export function projectMultiSelectItems(
  projects: SentryProject[],
  current: string[],
): SelectItemSpec[] {
  const items: SelectItemSpec[] = [];
  const seen = new Set<string>();
  for (const p of projects) {
    if (seen.has(p.slug)) continue;
    seen.add(p.slug);
    items.push({ id: p.slug, label: `${p.name} (${p.slug})` });
  }
  // Include currently-selected slugs even if the token can no longer see them
  // (editing an old watch, or a project since renamed/removed) so the picker
  // still shows the saved selection.
  for (const slug of current) {
    if (!slug || seen.has(slug)) continue;
    seen.add(slug);
    items.push({ id: slug, label: slug });
  }
  return items;
}

// isWatchFormReady aggregates the dialog's "can Save enable?" rule. Kept here
// so the rule has one named home and the dialog stays under its line limit.
export function isWatchFormReady(
  form: FormState,
  { requiresInstance = true }: { requiresInstance?: boolean } = {},
): boolean {
  return (
    !!form.workspaceId &&
    (!requiresInstance || !!form.sentryInstanceId) &&
    !!form.orgSlug.trim() &&
    form.projectSlugs.length > 0 &&
    !!form.workflowId &&
    !!form.workflowStepId &&
    !!form.prompt.trim() &&
    Number.isInteger(form.pollInterval) &&
    form.pollInterval >= 60 &&
    form.pollInterval <= 3600 &&
    parseMaxInflightTasks(form.maxInflightTasks) !== "invalid"
  );
}

export function buildFilterPayload(form: FormState): SentrySearchFilter {
  return {
    orgSlug: form.orgSlug.trim(),
    projectSlugs: form.projectSlugs.length > 0 ? form.projectSlugs : undefined,
    environment: form.environment.trim() || undefined,
    levels: form.levels.length > 0 ? form.levels : undefined,
    statuses: form.statuses.length > 0 ? form.statuses : undefined,
    query: form.query.trim() || undefined,
    statsPeriod: form.statsPeriod || undefined,
  };
}

// buildWatchPayload assembles the create/update watch fields shared by both
// paths (everything except workspaceId + sentryInstanceId, which only create
// carries). maxInflightTasks is resolved by the caller via parseMaxInflightTasks.
export function buildWatchPayload(form: FormState, maxInflightTasks: number | null) {
  return {
    filter: buildFilterPayload(form),
    workflowId: form.workflowId,
    workflowStepId: form.workflowStepId,
    // Empty repositoryId clears the binding; empty base branch is sent verbatim
    // so the backend fills the repo's default at save time.
    repositoryId: form.repositoryId,
    baseBranch: form.repositoryId ? form.baseBranch : "",
    agentProfileId: form.agentProfileId,
    executorProfileId: form.executorProfileId,
    prompt: form.prompt,
    enabled: form.enabled,
    pollIntervalSeconds: form.pollInterval,
    maxInflightTasks,
  };
}
