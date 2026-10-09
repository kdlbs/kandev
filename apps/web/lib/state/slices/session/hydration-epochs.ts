import type { SessionSliceState, TaskSessionHydrationEpoch } from "./types";

type SessionHydrationState = Pick<SessionSliceState, "taskSessions" | "taskSessionsByTask">;
type SessionEpochState = Pick<SessionSliceState, "taskSessions">;

/** Capture the live activity and read-cursor generations for one session. */
export function captureTaskSessionHydrationEpoch(
  state: SessionEpochState,
  sessionId: string,
): TaskSessionHydrationEpoch {
  return {
    activity: state.taskSessions.activityEpochBySession?.[sessionId] ?? 0,
    readCursor: state.taskSessions.readCursorEpochBySession?.[sessionId] ?? 0,
    workspaceRecovery: state.taskSessions.workspaceRecoveryEpochBySession?.[sessionId] ?? 0,
  };
}

/** Capture the live activity and read-cursor generations at list request start. */
export function captureTaskSessionHydrationEpochs(
  state: SessionHydrationState,
  taskId: string,
): Readonly<Record<string, TaskSessionHydrationEpoch>> {
  const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  return Object.fromEntries(
    sessions.map((session) => [session.id, captureTaskSessionHydrationEpoch(state, session.id)]),
  );
}
