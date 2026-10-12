import type { AutomationRun } from "@/lib/types/automation";

const MIN_RETRY_POLL_MS = 1_000;
const DEFAULT_RETRY_POLL_MS = 15_000;
const MAX_RETRY_POLL_MS = 60_000;
const RETRY_DUE_BUFFER_MS = 250;

export function retryPollDelay(runs: AutomationRun[], now = Date.now()): number {
  const scheduledAt = runs
    .filter((run) => run.status === "scheduled_retry" && run.retry_scheduled_at)
    .map((run) => Date.parse(run.retry_scheduled_at ?? ""))
    .filter(Number.isFinite);
  if (scheduledAt.length === 0) return DEFAULT_RETRY_POLL_MS;
  const earliestDue = Math.min(...scheduledAt);
  return Math.min(
    MAX_RETRY_POLL_MS,
    Math.max(MIN_RETRY_POLL_MS, earliestDue - now + RETRY_DUE_BUFFER_MS),
  );
}
