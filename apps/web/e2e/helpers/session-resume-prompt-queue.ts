import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient, QueueSessionIdentityInput } from "./api-client";
import { SessionPage } from "../pages/session-page";

export type DelayedResumeFixture = {
  task: CreateTaskResponse;
  session: SessionPage;
  identity: QueueSessionIdentityInput;
  delayedProfileId: string;
};

/** Create a mock-agent profile whose saved ACP session load fails on resume. */
export async function createFailOnResumeProfile(
  apiClient: ApiClient,
  name: string,
): Promise<{ id: string }> {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent not found while creating failed-resume profile");

  return apiClient.createAgentProfile(mockAgent.id, name, {
    model: "mock-fast",
    cli_passthrough: false,
    cli_flags: [{ description: "fail on ACP resume", flag: "--fail-on-resume", enabled: true }],
  });
}

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, { state?: string } | undefined> };
    };
  };
};

async function getBrowserSessionState(page: Page, sessionId: string): Promise<string | null> {
  return page.evaluate((id) => {
    const state = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState();
    return state?.taskSessions.items[id]?.state ?? null;
  }, sessionId);
}

async function getSessionState(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string | null> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId)?.state ?? null;
}

export async function waitForSessionStarting(
  page: Page,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  timeout = 15_000,
): Promise<void> {
  await expect
    .poll(
      async () => ({
        api: await getSessionState(apiClient, taskId, sessionId),
        browser: await getBrowserSessionState(page, sessionId),
      }),
      {
        timeout,
        message: `session ${sessionId} should remain in startup while resume is delayed`,
      },
    )
    .toEqual({ api: "STARTING", browser: "STARTING" });
}

export async function waitForSessionReady(
  page: Page,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  timeout = 60_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const api = await getSessionState(apiClient, taskId, sessionId);
        const browser = await getBrowserSessionState(page, sessionId);
        return {
          api: api !== null && api !== "STARTING",
          browser: browser !== null && browser !== "STARTING",
        };
      },
      {
        timeout,
        message: `session ${sessionId} should leave startup after the delayed resume completes`,
      },
    )
    .toEqual({ api: true, browser: true });
}

async function createDelayedResumeProfile(apiClient: ApiClient, delay = "30s"): Promise<string> {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent not found while creating delayed resume profile");

  const profile = await apiClient.createAgentProfile(
    mockAgent.id,
    `E2E delayed resume ${Date.now()}`,
    {
      model: "mock-fast",
      cli_passthrough: false,
      env_vars: [{ key: "E2E_MOCK_AGENT_RESUME_DELAY", value: delay }],
    },
  );
  return profile.id;
}

/** Seed an existing session, restart the backend, and stop during a delayed resume. */
export async function seedDelayedResumeFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
  title: string,
): Promise<DelayedResumeFixture> {
  const delayedProfileId = await createDelayedResumeProfile(apiClient);
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      delayedProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("delayed resume task has no session_id");

    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await backend.restart();
    await page.reload();
    await session.waitForLoad();
    await expect
      .poll(() => getSessionState(apiClient, task.id, task.session_id!), {
        timeout: 30_000,
        message: `session ${task.session_id} should enter startup after restart`,
      })
      .toBe("STARTING");
    await page.reload();
    await session.waitForLoad();
    await waitForSessionStarting(page, apiClient, task.id, task.session_id, 30_000);
    const identity = await apiClient.getQueueSessionIdentity(task.id, task.session_id);
    return { task, session, identity, delayedProfileId };
  } catch (error) {
    await apiClient.deleteAgentProfile(delayedProfileId, true).catch(() => undefined);
    throw error;
  }
}

export async function cleanupDelayedResumeFixture(
  apiClient: ApiClient,
  fixture: DelayedResumeFixture,
): Promise<void> {
  await apiClient.deleteAgentProfile(fixture.delayedProfileId, true).catch(() => undefined);
}

export async function waitForQueuedCount(
  apiClient: ApiClient,
  identity: QueueSessionIdentityInput,
  count: number,
  timeout = 20_000,
): Promise<void> {
  await expect
    .poll(() => apiClient.getQueueStatus(identity).then((status) => status.count), {
      timeout,
      message: `Waiting for ${count} queued prompt(s) for ${identity.sessionId}`,
    })
    .toBe(count);
}

export type SessionRuntimeIdentity = {
  executionId: string;
  acpSessionId: string;
};

/** Read the durable execution and provider conversation identities for a session. */
export async function readSessionRuntimeIdentity(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<SessionRuntimeIdentity> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId);
  const status = await apiClient.wsRequest<{ acp_session_id?: string }>("task.session.status", {
    task_id: taskId,
    session_id: sessionId,
  });
  if (!session?.agent_execution_id) {
    throw new Error(`Session ${sessionId} has no durable agent execution identity`);
  }
  if (!status.acp_session_id) {
    throw new Error(`Session ${sessionId} has no durable ACP conversation identity`);
  }
  return {
    executionId: session.agent_execution_id,
    acpSessionId: status.acp_session_id,
  };
}

/** Read message IDs whose persisted content contains a turn-specific marker. */
export async function readSessionMessageIdsContaining(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
): Promise<Set<string>> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return new Set(
    messages.filter((message) => message.content.includes(marker)).map((message) => message.id),
  );
}

/** Wait until a new persisted message contains the marker. */
export async function waitForNewSessionMessage(
  apiClient: ApiClient,
  sessionId: string,
  previousMessageIds: ReadonlySet<string>,
  marker: string,
  timeout = 30_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => !previousMessageIds.has(message.id) && message.content.includes(marker),
        );
      },
      {
        timeout,
        message: `Waiting for a new session message containing ${marker}`,
      },
    )
    .toBe(true);
}

/** Count persisted resume boot messages without relying on the UI deduplication. */
export async function countResumeBootMessages(
  apiClient: ApiClient,
  sessionId: string,
): Promise<number> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter(
    (message) =>
      message.metadata?.script_type === "agent_boot" && message.metadata?.is_resuming === true,
  ).length;
}
