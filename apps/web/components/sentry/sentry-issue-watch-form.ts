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
  };
}

// WatchDefaults carries the install-wide default org/project from the Sentry
// settings so the watcher can offer a "Use default" shortcut in each selector.
export type WatchDefaults = { orgSlug: string; projectSlug: string };

// USE_DEFAULT is the sentinel value for the "Use default" option. Selecting it
// resolves to the configured default slug (watches store a concrete org/project
// — the backend requires both, so there is no runtime fallback to defer to).
export const USE_DEFAULT = "__use_default__";

export type SelectItemSpec = { id: string; label: string };

export function orgSelectItems(
  orgs: string[],
  current: string,
  defaultOrgSlug: string,
): SelectItemSpec[] {
  const items: SelectItemSpec[] = [];
  if (defaultOrgSlug) {
    items.push({ id: USE_DEFAULT, label: `Use default (${defaultOrgSlug})` });
  }
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
  const items: SelectItemSpec[] = [];
  // Only offer the default project when it actually belongs to the selected org
  // (projects is already filtered by org) — otherwise it would point out of org.
  if (defaultProjectSlug && projects.some((p) => p.slug === defaultProjectSlug)) {
    items.push({ id: USE_DEFAULT, label: `Use default (${defaultProjectSlug})` });
  }
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
