import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const FAILED_STATES = ["FAILED", "CANCELLED"];

function sessionDoneOrThrow(session: { state: string; error_message?: string } | undefined) {
  if (!session) return false;
  if (FAILED_STATES.includes(session.state)) {
    throw new Error(
      `Session reached ${session.state}${session.error_message ? `: ${session.error_message}` : ""}`,
    );
  }
  return DONE_STATES.includes(session.state);
}

export async function waitForLatestSessionDone(
  apiClient: ApiClient,
  taskId: string,
  expectedCount: number,
  message: string,
  timeout = 120_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        if (sessions.length < expectedCount) return false;
        // API returns sessions newest-first.
        const latest = sessions[0];
        return sessionDoneOrThrow(latest);
      },
      { timeout, message },
    )
    .toBe(true);
}

export async function waitForSessionDone(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  message: string,
  timeout = 120_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((s) => s.id === sessionId);
        return sessionDoneOrThrow(session);
      },
      { timeout, message },
    )
    .toBe(true);
}

export async function waitForAgentMessage(
  apiClient: ApiClient,
  sessionId: string,
  content: string,
  timeout = 60_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(content),
        );
      },
      { timeout, message: `Waiting for agent message containing ${JSON.stringify(content)}` },
    )
    .toBe(true);
}

export async function waitForSessionState(
  apiClient: ApiClient,
  options: {
    taskId: string;
    sessionId: string;
    expectedState: string;
    message: string;
    timeout?: number;
  },
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(options.taskId);
        return sessions.find((session) => session.id === options.sessionId)?.state ?? null;
      },
      { timeout: options.timeout ?? 120_000, message: options.message },
    )
    .toBe(options.expectedState);
}

export async function waitForSessionEnvironment(
  apiClient: ApiClient,
  options: {
    taskId: string;
    sessionId: string;
    expectedEnvironmentId: string;
    message: string;
    timeout?: number;
  },
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(options.taskId);
        const session = sessions.find((s) => s.id === options.sessionId);
        return session?.task_environment_id ?? "";
      },
      { timeout: options.timeout ?? 60_000, message: options.message },
    )
    .toBe(options.expectedEnvironmentId);
}

export async function openTaskSession(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

/**
 * Seed a task + session and navigate to it, waiting for the first (normal)
 * turn to complete. Follow-up prompts can then exercise retry flows from a
 * clean idle state.
 */
export async function seedIdleSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  const session = await openTaskSession(testPage, task.id);
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.composerReady();
  return session;
}
