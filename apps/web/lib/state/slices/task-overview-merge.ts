import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import type { TaskOverview, TaskOverviewPatch, TaskOverviewState } from "./task-overview-types";

export function mergeTaskOverview(
  current: TaskOverview | undefined,
  patch: TaskOverviewPatch,
): TaskOverview {
  if (current === patch) return current;
  const before = parseStrictRfc3339Timestamp(current?.updatedAt?.replace(" ", "T"));
  const incoming = parseStrictRfc3339Timestamp(patch.updatedAt?.replace(" ", "T"));
  const stale = before !== null && incoming !== null && incoming < before;
  const next = { ...current, ...(stale ? {} : patch) } as TaskOverview;
  next.statusSummary = pickFreshestStatusSummary(patch.statusSummary, current?.statusSummary);
  if (!current) return next;
  // Reuse equal projection fields so an unchanged HTTP row keeps its canonical identity.
  for (const key of Object.keys(next) as Array<keyof TaskOverview>) {
    if (equalOverviewValue(next[key], current[key])) {
      Object.assign(next, { [key]: current[key] });
    }
  }
  const keys = Object.keys(next) as Array<keyof TaskOverview>;
  return keys.every((key) => Object.is(next[key], current[key])) &&
    Object.keys(current).every((key) => key in next)
    ? current
    : next;
}

function equalOverviewValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  return Boolean(
    left &&
    right &&
    typeof left === "object" &&
    typeof right === "object" &&
    JSON.stringify(left) === JSON.stringify(right),
  );
}

/** A bounded journal protects in-flight reads, including rows not yet resident. */
export function recordTaskOverviewChange(
  state: TaskOverviewState,
  taskId: string,
  patch: TaskOverviewPatch | null,
): TaskOverviewState {
  const reads = { ...state.reads };
  const byId = { ...state.byId };
  if (patch === null) delete byId[taskId];
  else if (byId[taskId]) byId[taskId] = mergeTaskOverview(byId[taskId], patch);
  let overflow = false;
  const encoder = new TextEncoder();
  for (const [id, read] of Object.entries(reads)) {
    const previous = read.changes[taskId];
    const change = patch === null || previous === null ? null : { ...previous, ...patch };
    const changes = { ...read.changes, [taskId]: change };
    const previousBytes = Object.hasOwn(read.changes, taskId)
      ? encoder.encode(JSON.stringify([taskId, previous])).byteLength
      : 0;
    const bytes =
      read.bytes - previousBytes + encoder.encode(JSON.stringify([taskId, change])).byteLength;
    if (Object.keys(changes).length > 1000 || bytes > 1024 * 1024) {
      delete reads[id];
      overflow = true;
    } else {
      reads[id] = { ...read, changes, bytes };
    }
  }
  return { ...state, byId, reads, generation: state.generation + Number(overflow) };
}

export function reconcileTaskOverviewRead(
  state: TaskOverviewState,
  tasks: TaskOverview[],
  readId?: string,
  includeLiveMembership = false,
): TaskOverview[] | null {
  const read = readId ? state.reads[readId] : undefined;
  if (readId && (!read || read.scope !== state.scope)) return null;
  const reconciled = tasks.flatMap((task) => {
    const change = read?.changes[task.id];
    if (change === null) return [];
    const merged = mergeTaskOverview(state.byId[task.id], task);
    return [change ? mergeTaskOverview(merged, change) : merged];
  });
  if (includeLiveMembership && read) {
    const ids = new Set(tasks.map((task) => task.id));
    for (const [id, change] of Object.entries(read.changes)) {
      if (change && !ids.has(id) && state.byId[id])
        reconciled.push(mergeTaskOverview(state.byId[id], change));
    }
  }
  return reconciled;
}
