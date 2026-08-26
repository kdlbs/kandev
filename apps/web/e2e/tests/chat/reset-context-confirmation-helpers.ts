import type { Page } from "@playwright/test";

import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

type ContextWindowStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      tasks: { activeSessionId: string | null };
      setContextWindow: (
        sessionId: string,
        contextWindow: {
          size: number;
          used: number;
          remaining: number;
          efficiency: number;
          compactionCount: number;
          source: "acp" | "api";
        },
      ) => void;
    };
  };
};

export async function seedResetContextSession(
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

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "reset-context session did not settle before navigation",
    30_000,
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

export async function seedStaleContextWindow(testPage: Page): Promise<void> {
  await testPage.evaluate(() => {
    const store = (window as ContextWindowStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");

    const sessionId = store.getState().tasks.activeSessionId;
    if (!sessionId) throw new Error("No active session is available");

    store.getState().setContextWindow(sessionId, {
      size: 200_000,
      used: 190_000,
      remaining: 10_000,
      efficiency: 95,
      compactionCount: 0,
      source: "acp",
    });
  });
}
