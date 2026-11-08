import type {
  NativeDashboard,
  NativeHistory,
  NativeLedger,
  NativeTaskList,
} from "@/lib/types/control-native";
import type { OverviewEvent } from "@/lib/types/control-overview";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

export const nativeRecord = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v);
const count = (v: unknown) => typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
const texts = (v: Record<string, unknown>, keys: string[]) =>
  keys.every((key) => typeof v[key] === "string");
const time = (v: unknown) => (typeof v === "string" ? parseStrictRfc3339Timestamp(v) : null);
function nativeWindow(v: Record<string, unknown>, id: string) {
  const end = time(v.generated_at),
    start = time(v.since);
  return (
    v.workspace_id === id &&
    end !== null &&
    start !== null &&
    end - start === BigInt("86400000000000")
  );
}
export function nativeHistory(v: unknown, id: string): v is NativeHistory {
  if (
    !nativeRecord(v) ||
    !nativeWindow(v, id) ||
    typeof v.coverage !== "string" ||
    typeof v.truncated !== "boolean" ||
    !nativeRecord(v.sources) ||
    !Array.isArray(v.events)
  )
    return false;
  const start = time(v.since)!,
    end = time(v.generated_at)!;
  const sources = Object.values(v.sources).every(
    (s) =>
      nativeRecord(s) &&
      ["known", "unavailable"].includes(String(s.state)) &&
      (s.total === null || count(s.total)) &&
      count(s.returned) &&
      typeof s.truncated === "boolean",
  );
  return (
    sources &&
    v.events.every(
      (e) =>
        nativeRecord(e) &&
        texts(e, ["id", "kind", "occurred_at"]) &&
        Boolean(e.id) &&
        time(e.occurred_at)! >= start &&
        time(e.occurred_at)! <= end &&
        time(e.occurred_at) !== null &&
        ["task_id", "session_id", "title", "state", "provider", "repository"].every(
          (key) => e[key] === undefined || typeof e[key] === "string",
        ) &&
        (e.number === undefined || (count(e.number) && Number(e.number) > 0)),
    )
  );
}
const sharedCapacityWaiting = (v: unknown) =>
  nativeRecord(v) && typeof v.known === "boolean" && count(v.grade) && count(v.total);

export function nativeDashboard(v: unknown, id: string): v is NativeDashboard {
  if (
    !nativeRecord(v) ||
    !nativeWindow(v, id) ||
    v.basis !== "recorded_task_session_queue_transition_state" ||
    v.inference_state !== "unknown"
  )
    return false;
  return (
    [v.tasks, v.sessions].every((m) => nativeRecord(m) && Object.values(m).every(count)) &&
    [v.waiting_input_tasks, v.queued_messages, v.transitions_24h, v.completed_tasks_24h].every(
      count,
    ) &&
    nativeDashboardOptionals(v)
  );
}
/** Fields older servers omit; present values must still be well-formed. */
function nativeDashboardOptionals(v: Record<string, unknown>) {
  const stages = v.task_stages;
  return (
    (v.wip_waiting_tasks === undefined || count(v.wip_waiting_tasks)) &&
    (stages === undefined ||
      (Array.isArray(stages) &&
        stages.every(
          (row) =>
            nativeRecord(row) &&
            texts(row, ["state", "step_id", "step_name", "stage_type"]) &&
            count(row.count),
        ))) &&
    (v.shared_capacity_waiting === undefined || sharedCapacityWaiting(v.shared_capacity_waiting)) &&
    (v.merged_change_requests_24h == null || count(v.merged_change_requests_24h))
  );
}
export function nativeTasks(
  v: unknown,
  id: string,
  filter: string,
  archive: string,
): v is NativeTaskList {
  if (
    !nativeRecord(v) ||
    v.kind !== "tasks" ||
    v.filter !== filter ||
    v.archive !== archive ||
    !count(v.total) ||
    !Array.isArray(v.tasks) ||
    v.tasks.length > Number(v.total) ||
    typeof v.truncated !== "boolean" ||
    time(v.generated_at) === null
  )
    return false;
  return v.tasks.every(
    (r) =>
      nativeRecord(r) &&
      r.workspace_id === id &&
      texts(r, [
        "task_id",
        "title",
        "state",
        "step_id",
        "step_name",
        "stage_type",
        "session_id",
        "session_state",
        "updated_at",
      ]) &&
      Boolean(r.task_id) &&
      time(r.updated_at) !== null &&
      typeof r.archived === "boolean" &&
      count(r.running_sessions) &&
      count(r.queued_messages) &&
      (r.hold_mode === undefined || typeof r.hold_mode === "string") &&
      (r.queued_paused_messages === undefined || count(r.queued_paused_messages)) &&
      (r.queued_auto_run_messages === undefined || count(r.queued_auto_run_messages)),
  );
}
export function nativeLedger(v: unknown, id: string): v is NativeLedger {
  if (
    !nativeRecord(v) ||
    !nativeWindow(v, id) ||
    v.basis !== "task_usage_events" ||
    v.account_attribution !== "unavailable" ||
    typeof v.coverage !== "string" ||
    typeof v.truncated !== "boolean" ||
    !Array.isArray(v.models)
  )
    return false;
  return v.models.every(
    (r) =>
      nativeRecord(r) &&
      texts(r, ["model", "provider", "measurement_source", "usage_completeness", "cost_source"]) &&
      typeof r.estimated === "boolean" &&
      [r.event_count, r.tokens_in, r.tokens_total, r.cost_subcents].every(count) &&
      [r.tokens_out, r.tokens_cached_read, r.tokens_cached_write].every(
        (n) => n === null || count(n),
      ),
  );
}
const KINDS: Record<string, string> = {
  task_moved: "step_move",
  automation_run_created: "automation_run",
  github_merged: "pr_merged",
  plugin_merged: "pr_merged",
};
export function nativeEvents(data: NativeHistory): OverviewEvent[] {
  return data.events.map(
    (e) =>
      ({
        id: e.id,
        kind: KINDS[e.kind] ?? e.kind,
        at: e.occurred_at,
        workspace_id: data.workspace_id,
        task_id: e.task_id,
        session_id: e.session_id,
        title: e.title,
        recorded_state: e.state,
        detail: e.provider
          ? [e.provider, e.repository, e.number ? `#${e.number}` : undefined]
              .filter(Boolean)
              .join(" ")
          : undefined,
      }) as OverviewEvent,
  );
}
