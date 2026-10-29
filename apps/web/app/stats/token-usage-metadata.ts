import type { TokenUsageMetrics, UsageMetricSource } from "@/lib/types/http";
import type { TokenUsageAggregate, TokenUsageView } from "./token-usage-utils";
import { formatTokenPeriod } from "./token-usage-utils";

const UNKNOWN_KEY = "common:unknown";

export type UsageTranslate = (key: string, options?: Record<string, unknown>) => string;

export function actorAttributionLabel(
  source: UsageMetricSource | null | undefined,
  t: UsageTranslate,
): string {
  if (source?.actor_status === "mixed") return t("stats:mixedUsers");
  if (source?.actor_status === "ambiguous") return t("stats:ambiguousActor");
  if (source?.actor_status !== "attributed" || !source.actor_id) return t(UNKNOWN_KEY);
  const label = actorKindLabel(source.actor_kind);
  return label ? t(label, { id: source.actor_id }) : t(UNKNOWN_KEY);
}

function actorKindLabel(kind: string | undefined): string | null {
  if (kind === "user") return "stats:attributedUser";
  if (kind === "service") return "stats:serviceActor";
  if (kind === "synthetic") return "stats:syntheticActor";
  return null;
}

export function usageProviderLabel(provider: string | undefined, t: UsageTranslate): string {
  if (provider === "mixed") return t("stats:mixedProviders");
  return provider && provider !== "__unknown_provider__" ? provider : t(UNKNOWN_KEY);
}

export function usageCalendarLabel(metrics: TokenUsageMetrics, t: UsageTranslate): string {
  if (metrics.calendar_basis === "mixed") return t("stats:calendarMixed");
  return metrics.calendar || t("common:unavailable");
}

export function usageCurrencyLabel(currency: string, t: UsageTranslate): string {
  return currency === "mixed" ? t("stats:multipleCurrenciesNotice") : currency || t(UNKNOWN_KEY);
}

export function isUsageUnallocated(
  row: Pick<TokenUsageAggregate, "task_id" | "session_id">,
  view: TokenUsageView,
): boolean {
  return (view === "tasks" && !row.task_id) || (view === "sessions" && !row.session_id);
}

export function usageRowLabel(
  row: TokenUsageAggregate,
  view: TokenUsageView,
  t: UsageTranslate,
): string {
  if (isUsageUnallocated(row, view)) return t("stats:unallocatedUsage");
  if (view === "tasks" && row.task_available === false) return t("stats:deletedTask");
  if (view === "sessions" && row.session_available === false) return t("stats:unavailableSession");
  if (view === "daily" || view === "monthly")
    return row.label ? formatTokenPeriod(row.label, view) : t("stats:undatedCoverageOnly");
  return row.label || t(UNKNOWN_KEY);
}

export function usageTaskDestination(row: TokenUsageAggregate, view: TokenUsageView) {
  if (!row.task_available || !row.task_id) return null;
  if (view === "tasks") return { taskId: row.task_id };
  if (view === "sessions" && row.session_available && row.session_id)
    return { taskId: row.task_id, sessionId: row.session_id };
  return null;
}
