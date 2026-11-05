import type { NativeDashboard, NativeTaskList } from "@/lib/types/control-native";
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
    (v.task_stages === undefined ||
      (Array.isArray(v.task_stages) &&
        v.task_stages.every(
          (row) =>
            nativeRecord(row) &&
            texts(row, ["state", "step_id", "step_name", "stage_type"]) &&
            count(row.count),
        )))
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
