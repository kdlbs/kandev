import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { ReviewFindingStatus, TaskReviewFinding } from "@/lib/types/review";

type ActionGroup = {
  latest: number;
  latestPending: boolean;
  acknowledged: number;
  baseline: TaskReviewFinding;
  owned: TaskReviewFinding;
  pending: number;
};

const groups = new WeakMap<StoreApi<AppState>, Map<string, Map<string, ActionGroup>>>();

function findingGroups(store: StoreApi<AppState>, taskId: string) {
  let tasks = groups.get(store);
  if (!tasks) {
    tasks = new Map();
    groups.set(store, tasks);
  }
  let findings = tasks.get(taskId);
  if (!findings) {
    findings = new Map();
    tasks.set(taskId, findings);
  }
  return { tasks, findings };
}

/** Local action completions publish only while their group owns the displayed row. */
export function beginFindingAction(
  store: StoreApi<AppState>,
  taskId: string,
  findingId: string,
  status: ReviewFindingStatus,
) {
  const currentRow = () =>
    store.getState().taskReview.findingsByTaskId[taskId]?.find((row) => row.id === findingId);
  const current = currentRow();
  if (!current) return;
  const { tasks, findings } = findingGroups(store, taskId);
  let group = findings.get(findingId);
  if (!group || group.owned !== current) {
    group = {
      latest: 0,
      latestPending: false,
      acknowledged: 0,
      baseline: current,
      owned: current,
      pending: 0,
    };
    findings.set(findingId, group);
  }
  const owner = group;
  const ordinal = ++owner.latest;
  owner.latestPending = true;
  owner.pending++;

  const publish = (row: TaskReviewFinding) => {
    // Set ownership before notifying subscribers; independent replacements retain authority.
    owner.owned = row;
    store.getState().updateReviewFinding(taskId, row);
  };
  publish({ ...current, status });

  return (acknowledgement?: TaskReviewFinding) => {
    if (findings.get(findingId) === owner && currentRow() === owner.owned) {
      if (ordinal === owner.latest) owner.latestPending = false;
      if (acknowledgement && ordinal > owner.acknowledged) {
        owner.acknowledged = ordinal;
        owner.baseline = acknowledgement;
        if (!owner.latestPending) publish(owner.baseline);
      } else if (!acknowledgement && ordinal === owner.latest) {
        publish(owner.baseline);
      }
    }
    owner.pending--;
    if (owner.pending || findings.get(findingId) !== owner) return;
    findings.delete(findingId);
    if (findings.size === 0) tasks.delete(taskId);
    if (tasks.size === 0) groups.delete(store);
  };
}
