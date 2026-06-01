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
    orgSlug: f.orgSlug ?? "",
    projectSlug: f.projectSlug ?? "",
    environment: f.environment ?? "",
    levels: (f.levels ?? []) as SentryLevel[],
    statuses: (f.statuses ?? []) as SentryStatus[],
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

// WatchDefaults carries the install-wide default org/project from the Sentry
// settings, used to label the always-present "Use default" option.
export type WatchDefaults = { orgSlug: string; projectSlug: string };

// USE_DEFAULT is the sentinel for the "Use default" option, mirroring
// STEP_DEFAULT for the profile selects. The form stores "" to mean "follow the
// install-wide default org/project"; the backend resolves "" fresh on every
// poll (resolveWatchFilter). Radix disallows <SelectItem value="">, so the
// dropdown item carries this sentinel and maps back to "" on change.
export const USE_DEFAULT = "__use_default__";

export type SelectItemSpec = { id: string; label: string };

// defaultSelectItem builds the "Use default" row, always present so the option
// exists even before an install-wide default has been configured.
function defaultSelectItem(defaultSlug: string): SelectItemSpec {
  return { id: USE_DEFAULT, label: defaultSlug ? `Use default (${defaultSlug})` : "Use default" };
}

// resolveSlugSelection maps a select value back to the stored slug, collapsing
// the sentinel to "" so the payload keeps signalling "use default".
export function resolveSlugSelection(value: string): string {
  return value === USE_DEFAULT ? "" : value;
}

export function orgSelectItems(
  orgs: string[],
  current: string,
  defaultOrgSlug: string,
): SelectItemSpec[] {
  const items: SelectItemSpec[] = [defaultSelectItem(defaultOrgSlug)];
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

export function projectSelectItems(
  projects: SentryProject[],
  current: string,
  defaultProjectSlug: string,
): SelectItemSpec[] {
  const items: SelectItemSpec[] = [defaultSelectItem(defaultProjectSlug)];
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
