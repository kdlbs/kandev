import { expect } from "@playwright/test";

import type { ApiClient } from "./api-client";
import type { SessionStatusRequestFrame } from "./ws-capture";

/** Count status requests for one exact task/session pair. */
export function countSessionStatusRequests(
  frames: SessionStatusRequestFrame[],
  taskId: string,
  sessionId: string,
): number {
  return frames.filter((frame) => frame.taskId === taskId && frame.sessionId === sessionId).length;
}

/**
 * Assert every observed `task.session.status` request names a task that owns
 * the requested session. `extraSessions` covers a just-created session that a
 * concurrent list read may not include yet.
 */
export async function expectOwnedSessionStatusRequests(
  apiClient: Pick<ApiClient, "listTaskSessions">,
  frames: SessionStatusRequestFrame[],
  tasks: Array<{ taskId: string; sessionIds?: string[] }>,
): Promise<void> {
  const ownerBySession = new Map<string, string>();
  for (const task of tasks) {
    for (const sessionId of task.sessionIds ?? []) ownerBySession.set(sessionId, task.taskId);
    const response = await apiClient.listTaskSessions(task.taskId);
    for (const session of response.sessions) ownerBySession.set(session.id, task.taskId);
  }
  const foreign = frames.filter((frame) => ownerBySession.get(frame.sessionId) !== frame.taskId);
  expect(foreign, `foreign task/session status requests: ${JSON.stringify(foreign)}`).toEqual([]);
}
