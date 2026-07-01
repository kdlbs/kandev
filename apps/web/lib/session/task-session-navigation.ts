import type { TaskSession } from "@/lib/types/http";

type TaskSwitchSessionInput = Pick<
  TaskSession,
  "agent_profile_snapshot" | "is_passthrough" | "task_environment_id"
>;

export function sessionHasRoutingInfo(
  session: Pick<TaskSession, "agent_profile_snapshot" | "is_passthrough"> | null | undefined,
): boolean {
  return Boolean(
    session &&
    (session.is_passthrough !== undefined || session.agent_profile_snapshot !== undefined),
  );
}

export function taskSessionsAreNavigationReady(sessions: TaskSwitchSessionInput[]): boolean {
  return (
    sessions.length === 0 ||
    sessions.every((session) => !!session.task_environment_id && sessionHasRoutingInfo(session))
  );
}
