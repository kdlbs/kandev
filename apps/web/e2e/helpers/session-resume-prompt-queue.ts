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
