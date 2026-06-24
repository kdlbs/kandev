import type {
  SentryIssueWatch,
  SentryLevel,
  SentryProject,
  SentrySearchFilter,
  SentryStatus,
} from "@/lib/types/sentry";
import { DEFAULT_SENTRY_ISSUE_WATCH_PROMPT } from "./sentry-issue-watch-placeholders";

export const LEVEL_OPTIONS: SentryLevel[] = ["fatal", "error", "warning", "info", "debug"];
export const STATUS_OPTIONS: SentryStatus[] = ["unresolved", "resolved", "ignored"];

export const STATS_PERIOD_OPTIONS: { value: string; label: string }[] = [
  { value: "1h", label: "Last hour" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
  { value: "14d", label: "Last 14 days" },
  { value: "30d", label: "Last 30 days" },
];

export interface FormState {
  workspaceId: string;
  instanceId: string;
  orgSlug: string;
  projectSlug: string;
  environment: string;
  levels: SentryLevel[];
  statuses: SentryStatus[];
  query: string;
  statsPeriod: string;
  workflowId: string;
  workflowStepId: string;
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
    instanceId: "",
    orgSlug: "",
    projectSlug: "",
    environment: "",
    levels: ["fatal", "error"],
    statuses: ["unresolved"],
    query: "",
    statsPeriod: "24h",
    workflowId: "",
    workflowStepId: "",
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
    instanceId: w.instanceId,
    orgSlug: f.orgSlug ?? "",
    projectSlug: f.projectSlug ?? "",
    environment: f.environment ?? "",
    levels: f.levels ?? [],
    statuses: f.statuses ?? [],
    query: f.query ?? "",
    statsPeriod: f.statsPeriod ?? "",
    workflowId: w.workflowId,
    workflowStepId: w.workflowStepId,
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

export function projectSelectItems(projects: SentryProject[], current: string): SelectItemSpec[] {
  const items: SelectItemSpec[] = [];
  const seen = new Set<string>();
  for (const p of projects) {
    if (seen.has(p.slug)) continue;
    seen.add(p.slug);
    items.push({ id: p.slug, label: `${p.name} (${p.slug})` });
  }
  if (current && !seen.has(current)) {
    items.push({ id: current, label: current });
  }
  return items;
}

// isWatchFormReady aggregates the dialog's "can Save enable?" rule. Kept here
// so the rule has one named home and the dialog stays under its line limit.
export function isWatchFormReady(form: FormState): boolean {
  return (
    !!form.workspaceId &&
    !!form.instanceId &&
    !!form.orgSlug.trim() &&
    !!form.projectSlug.trim() &&
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
    projectSlug: form.projectSlug.trim() || undefined,
    environment: form.environment.trim() || undefined,
    levels: form.levels.length > 0 ? form.levels : undefined,
    statuses: form.statuses.length > 0 ? form.statuses : undefined,
    query: form.query.trim() || undefined,
    statsPeriod: form.statsPeriod || undefined,
  };
}
