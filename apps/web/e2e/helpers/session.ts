import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

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
        return DONE_STATES.includes(latest?.state ?? "");
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
        return DONE_STATES.includes(session?.state ?? "");
      },
      { timeout, message },
    )
    .toBe(true);
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
