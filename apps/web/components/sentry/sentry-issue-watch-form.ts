import type {
  SentryIssueWatch,
  SentryLevel,
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
