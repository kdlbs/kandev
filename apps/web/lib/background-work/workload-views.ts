import type { WorkloadRunObservation } from "@/lib/types/background-work";

export type BackgroundWorkTab = "active" | "history";

/** True for the workloads the composer chip counts and summarizes. */
export function isRunningWorkload(workload: WorkloadRunObservation): boolean {
  return workload.state === "running";
}

/** True for the workloads the overview Active tab lists. */
export function isActiveWorkload(workload: WorkloadRunObservation): boolean {
  return workload.state === "running" || workload.state === "waiting";
}

/** True for the workloads the overview History tab lists. Unknown is history,
 *  not terminal: its actual status is preserved and it can return to Active. */
export function isHistoryWorkload(workload: WorkloadRunObservation): boolean {
  return !isActiveWorkload(workload);
}

/**
 * Keeps the accepted list intact while guaranteeing one row per work id. A
 * newer revision wins, and first-seen order is preserved for stable rendering.
 */
export function uniqueWorkloads(
  workloads: readonly WorkloadRunObservation[],
): WorkloadRunObservation[] {
  const accepted = new Map<string, WorkloadRunObservation>();
  for (const workload of workloads) {
    const previous = accepted.get(workload.work_id);
    if (!previous || workload.revision >= previous.revision) {
      accepted.set(workload.work_id, workload);
    }
  }
  return [...accepted.values()];
}

export function selectRunningWorkloads(
  workloads: readonly WorkloadRunObservation[],
): WorkloadRunObservation[] {
  return uniqueWorkloads(workloads).filter(isRunningWorkload);
}

export function selectActiveWorkloads(
  workloads: readonly WorkloadRunObservation[],
): WorkloadRunObservation[] {
  return uniqueWorkloads(workloads).filter(isActiveWorkload);
}

export function selectHistoryWorkloads(
  workloads: readonly WorkloadRunObservation[],
): WorkloadRunObservation[] {
  return uniqueWorkloads(workloads).filter(isHistoryWorkload);
}

export function selectWorkloadsForTab(
  workloads: readonly WorkloadRunObservation[],
  tab: BackgroundWorkTab,
): WorkloadRunObservation[] {
  return tab === "active" ? selectActiveWorkloads(workloads) : selectHistoryWorkloads(workloads);
}

/** Initial overview tab once the first fetch settles. */
export function resolveInitialBackgroundWorkTab(
  workloads: readonly WorkloadRunObservation[],
): BackgroundWorkTab {
  return selectActiveWorkloads(workloads).length > 0 ? "active" : "history";
}

/** Case-insensitive title/kind search. An empty query keeps every row. */
export function filterWorkloadsByQuery(
  workloads: readonly WorkloadRunObservation[],
  query: string,
): WorkloadRunObservation[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return [...workloads];
  return workloads.filter(
    (workload) =>
      workload.title.toLowerCase().includes(normalized) ||
      workload.kind.toLowerCase().includes(normalized),
  );
}
