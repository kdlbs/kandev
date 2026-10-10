import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { GatewayTrafficFrame } from "./ws-traffic";

type SessionRecord = Awaited<ReturnType<ApiClient["listTaskSessions"]>>["sessions"][number];

export type SilentRestartSnapshot = {
  sessionId: string;
  nativeSessionId: string;
  agentExecutionId?: string;
  taskEnvironmentId?: string;
  messageIds: string[];
  turnIds: string[];
  userMessages: Array<{ id: string; content: string; turn_id?: string }>;
  queuedMessageCount: number;
};

export type RestartExecutionDisposition = "preserved" | "replaced";

function nativeSessionId(session: SessionRecord): string {
  const acp = session.metadata?.acp;
  const id =
    typeof acp === "object" && acp !== null
      ? (acp as Record<string, unknown>).session_id
      : undefined;
  if (typeof id !== "string" || id.length === 0) {
    throw new Error(`session ${session.id} has no retained ACP session ID`);
  }
  return id;
}

async function getSession(api: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await api.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId);
  if (!session) throw new Error(`task ${taskId} no longer has session ${sessionId}`);
  return session;
}

export async function captureSilentRestartSnapshot(
  api: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<SilentRestartSnapshot> {
  const session = await getSession(api, taskId, sessionId);
  const [messages, turns, queueIdentity] = await Promise.all([
    api.listSessionMessages(sessionId),
    api.listSessionTurns(sessionId),
    api.getQueueSessionIdentity(taskId, sessionId),
  ]);
  const queue = await api.getQueueStatus(queueIdentity);
  return {
    sessionId,
    nativeSessionId: nativeSessionId(session),
    agentExecutionId: session.agent_execution_id,
    taskEnvironmentId: session.task_environment_id,
    messageIds: messages.messages.map((message) => message.id),
    turnIds: turns.turns.map((turn) => turn.id),
    userMessages: messages.messages
      .filter((message) => message.author_type === "user")
      .map(({ id, content, turn_id }) => ({ id, content, turn_id })),
    queuedMessageCount: queue.count,
  };
}

export async function expectSilentRestartRestoration(
  page: Page,
  api: ApiClient,
  taskId: string,
  snapshot: SilentRestartSnapshot,
  traffic: readonly GatewayTrafficFrame[],
): Promise<void> {
  await expectRestartConversationIdentity({
    page,
    api,
    taskId,
    snapshot,
    traffic,
    expectedState: "WAITING_FOR_INPUT",
    executionDisposition: "replaced",
  });

  await expect
    .poll(
      async () => {
        const restored = await getSession(api, taskId, snapshot.sessionId);
        const recovery = restored.metadata?.agent_delivery_recovery as
          | { phase?: unknown }
          | undefined;
        return typeof recovery?.phase === "string" ? recovery.phase : "";
      },
      {
        timeout: 60_000,
        message: "session delivery recovery was not durably committed after backend restart",
      },
    )
    .toBe("restored");

  for (const testId of ["failed-session-banner", "delivery-recovery-result"]) {
    await expect(page.getByTestId(testId)).toHaveCount(0);
  }
}

export async function expectRestartConversationIdentity({
  page,
  api,
  taskId,
  snapshot,
  traffic,
  expectedState,
  executionDisposition,
}: {
  page: Page;
  api: ApiClient;
  taskId: string;
  snapshot: SilentRestartSnapshot;
  traffic: readonly GatewayTrafficFrame[];
  expectedState: "RUNNING" | "WAITING_FOR_INPUT";
  executionDisposition: RestartExecutionDisposition;
}): Promise<void> {
  await expect
    .poll(async () => (await getSession(api, taskId, snapshot.sessionId)).state, {
      timeout: 60_000,
      message: `session did not return to ${expectedState} after backend restart`,
    })
    .toBe(expectedState);

  const restored = await getSession(api, taskId, snapshot.sessionId);
  expect(restored.id).toBe(snapshot.sessionId);
  expect(restored.task_environment_id).toBe(snapshot.taskEnvironmentId);
  if (executionDisposition === "preserved") {
    expect(snapshot.agentExecutionId).toBeTruthy();
    expect(restored.agent_execution_id).toBe(snapshot.agentExecutionId);
  } else {
    expect(snapshot.agentExecutionId).toBeTruthy();
    expect(restored.agent_execution_id).toBeTruthy();
    expect(restored.agent_execution_id).not.toBe(snapshot.agentExecutionId);
  }
  expect(nativeSessionId(restored)).toBe(snapshot.nativeSessionId);

  const [messages, turns, queueIdentity] = await Promise.all([
    api.listSessionMessages(snapshot.sessionId),
    api.listSessionTurns(snapshot.sessionId),
    api.getQueueSessionIdentity(taskId, snapshot.sessionId),
  ]);
  const queue = await api.getQueueStatus(queueIdentity);
  const messageIdsAfter = messages.messages.map((message) => message.id);
  expect(new Set(messageIdsAfter).size).toBe(messageIdsAfter.length);
  for (const messageId of snapshot.messageIds) expect(messageIdsAfter).toContain(messageId);
  expect(
    messages.messages
      .filter((message) => message.author_type === "user")
      .map(({ id, content, turn_id }) => ({ id, content, turn_id })),
  ).toEqual(snapshot.userMessages);
  expect(turns.turns.map((turn) => turn.id)).toEqual(snapshot.turnIds);
  expect(queue.count).toBe(snapshot.queuedMessageCount);

  expect(
    traffic.filter(
      (frame) =>
        frame.direction === "sent" &&
        frame.action === "session.recover" &&
        frame.sessionId === snapshot.sessionId,
    ),
  ).toHaveLength(0);

  for (const testId of [
    "interrupted-sessions-notice",
    "interrupted-sessions-open",
    "interrupted-batch-instruction",
    "interrupted-session-continuation",
    "interrupted-recovery-instruction",
  ]) {
    await expect(page.getByTestId(testId)).toHaveCount(0);
  }
}

export function countSessionMessageAdds(
  frames: readonly { taskId: string; sessionId: string; content: string }[],
  taskId: string,
  sessionId: string,
): number {
  return frames.filter((frame) => frame.taskId === taskId && frame.sessionId === sessionId).length;
}
