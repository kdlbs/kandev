import type { OverviewEvent } from "@/lib/types/control-overview";

export const STEP_MOVE_KIND = "step_move";
/** A task moving this many times inside the window is flagged as repeating. */
export const REPEAT_THRESHOLD = 10;
export const REPEAT_WINDOW_MS = 60 * 60_000;

export type RepeatedMoves = {
  taskId: string;
  workspaceId?: string;
  title?: string;
  count: number;
  start: string;
  end: string;
};

const at = (event: OverviewEvent) => Date.parse(event.at);

/**
 * Tasks whose step moves reach the threshold inside any window of the given
 * length. The densest window is reported, with its first and last move.
 */
export function repeatedMoves(
  events: OverviewEvent[],
  threshold = REPEAT_THRESHOLD,
  windowMs = REPEAT_WINDOW_MS,
): Map<string, RepeatedMoves> {
  const byTask = new Map<string, OverviewEvent[]>();
  for (const event of events) {
    if (event.kind !== STEP_MOVE_KIND || !event.task_id || Number.isNaN(at(event))) continue;
    byTask.set(event.task_id, [...(byTask.get(event.task_id) ?? []), event]);
  }
  const result = new Map<string, RepeatedMoves>();
  for (const [taskId, moves] of byTask) {
    const sorted = [...moves].sort((a, b) => at(a) - at(b));
    let best = { count: 0, first: 0, last: 0 };
    let first = 0;
    for (let last = 0; last < sorted.length; last++) {
      while (at(sorted[last]) - at(sorted[first]) > windowMs) first++;
      if (last - first + 1 > best.count) best = { count: last - first + 1, first, last };
    }
    if (best.count < threshold) continue;
    const latest = sorted[sorted.length - 1];
    result.set(taskId, {
      taskId,
      workspaceId: latest.workspace_id,
      title: latest.title,
      count: best.count,
      start: sorted[best.first].at,
      end: sorted[best.last].at,
    });
  }
  return result;
}

export type HistoryItem =
  | { type: "event"; event: OverviewEvent }
  | { type: "moves"; taskId: string; events: OverviewEvent[]; start: string; end: string };

/**
 * Folds a task's consecutive step moves into one item. `events` must be newest
 * first. Moves stay in one run while no other event of the same task falls
 * between them and each gap is at most `gapMs`; the run sits where its newest
 * move was. A run of one move stays a plain event.
 */
export function historyItems(events: OverviewEvent[], gapMs = REPEAT_WINDOW_MS): HistoryItem[] {
  const items: HistoryItem[] = [];
  const open = new Map<string, Extract<HistoryItem, { type: "moves" }>>();
  for (const event of events) {
    const taskId = event.task_id;
    if (event.kind !== STEP_MOVE_KIND || !taskId) {
      if (taskId) open.delete(taskId);
      items.push({ type: "event", event });
      continue;
    }
    const run = open.get(taskId);
    if (run && at(run.events[run.events.length - 1]) - at(event) <= gapMs) {
      run.events.push(event);
      run.start = event.at;
      continue;
    }
    const next = {
      type: "moves" as const,
      taskId,
      events: [event],
      start: event.at,
      end: event.at,
    };
    open.set(taskId, next);
    items.push(next);
  }
  return items.map((item) =>
    item.type === "moves" && item.events.length === 1
      ? { type: "event", event: item.events[0] }
      : item,
  );
}
