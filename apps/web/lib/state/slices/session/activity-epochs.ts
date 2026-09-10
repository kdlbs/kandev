import type { SessionSliceState } from "./types";

type SessionActivityState = Pick<SessionSliceState, "taskSessions" | "taskSessionsByTask">;

/** Capture the live-activity generations that an asynchronous list request starts from. */
export function captureTaskSessionActivityEpochs(
  state: SessionActivityState,
  taskId: string,
): Readonly<Record<string, number>> {
  const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  return Object.fromEntries(
    sessions.map((session) => [
      session.id,
      state.taskSessions.activityEpochBySession?.[session.id] ?? 0,
    ]),
  );
}
