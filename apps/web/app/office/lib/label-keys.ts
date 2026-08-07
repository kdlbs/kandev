import type {
  OfficeTaskPriority,
  OfficeTaskStatus,
  ProjectStatus,
  Tier,
} from "@/lib/state/slices/office/types";

/**
 * Catalog keys for the office task status, task priority and project status
 * vocabularies.
 *
 * Keys, not resolved labels: these are module-scope tables, so a `t()` here
 * would resolve once at import and freeze at the boot locale. Every consumer
 * resolves at render.
 *
 * The record keys are the wire `OfficeTaskStatus` / `OfficeTaskPriority` values
 * and are never translated — they are compared, persisted and sent to the
 * backend. Shared so the status icon, the board columns, the filter list and the
 * new-task chips cannot drift apart; each of them previously carried its own
 * copy of the same English labels.
 *
 * These are FALLBACKS. When `office.meta` is loaded, the workspace's own status
 * and priority labels win, and those are workspace data rather than copy.
 */
export const STATUS_LABEL_KEYS: Record<OfficeTaskStatus, string> = {
  backlog: "office:statusBacklog",
  todo: "office:statusTodo",
  in_progress: "office:statusInProgress",
  in_review: "office:statusInReview",
  blocked: "office:statusBlocked",
  done: "office:statusDone",
  cancelled: "office:statusCancelled",
};

export const PRIORITY_LABEL_KEYS: Record<OfficeTaskPriority, string> = {
  critical: "office:priorityCritical",
  high: "office:priorityHigh",
  medium: "office:priorityMedium",
  low: "office:priorityLow",
  none: "office:none",
};

export const PROJECT_STATUS_LABEL_KEYS: Record<ProjectStatus, string> = {
  active: "office:projectStatusActive",
  completed: "office:projectStatusCompleted",
  on_hold: "office:projectStatusOnHold",
  archived: "office:projectStatusArchived",
};

/**
 * Lowercase tier names for use inside a sentence ("as the frontier tier"). The
 * Tier ids are wire values; the toggle group's own capitalized labels live under
 * `office:tier*` and are a different register, so they are not shared.
 */
export const TIER_NAME_KEYS: Record<Tier, string> = {
  frontier: "office:tierNameFrontier",
  balanced: "office:tierNameBalanced",
  economy: "office:tierNameEconomy",
};

/**
 * Routine concurrency policies. The row and the expanded detail used to render
 * the wire value with `policy.replace(/_/g, " ")`, which showed the identifier
 * as pseudo-English and could never be localized.
 */
export const CONCURRENCY_POLICY_LABEL_KEYS: Record<string, string> = {
  skip_if_active: "office:skipIfActive",
  coalesce_if_active: "office:coalesceIfActive",
  always_create: "office:alwaysCreate",
};
