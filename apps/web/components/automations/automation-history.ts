import type { AutomationRun, RetryHistoryMode, RetryHistoryPage } from "@/lib/types/automation";

export function projectAutomationHistory(
  history: AutomationRun[] | RetryHistoryPage,
  mode: RetryHistoryMode = "attempts",
): AutomationRun[] {
  const runs = Array.isArray(history)
    ? history
    : (history.items ?? []).flatMap((item) => item.attempts ?? []);
  if (mode === "attempts") return runs;
  const groups = new Map<string, AutomationRun>();
  for (const run of runs) {
    const key = run.retry_group_id ? `retry:${run.retry_group_id}` : `legacy:${run.id}`;
    const current = groups.get(key);
    if (!current || (run.attempt_number ?? 1) > (current.attempt_number ?? 1)) {
      groups.set(key, run);
    }
  }
  return [...groups.values()].sort((left, right) =>
    right.created_at.localeCompare(left.created_at),
  );
}

export function expandRetryGroupRunIDs(
  allRuns: AutomationRun[],
  visibleRuns: AutomationRun[],
): string[] {
  const visibleIDs = new Set(visibleRuns.map((run) => run.id));
  const visibleGroups = new Set(
    visibleRuns.flatMap((run) => (run.retry_group_id ? [run.retry_group_id] : [])),
  );
  return allRuns
    .filter(
      (run) =>
        visibleIDs.has(run.id) || (run.retry_group_id && visibleGroups.has(run.retry_group_id)),
    )
    .map((run) => run.id);
}
