import type { Locator, Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type ContextWindowStore = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      setContextWindow: (
        sessionId: string,
        contextWindow: {
          size: number;
          used: number;
          remaining: number;
          efficiency: number;
          source: "acp" | "api";
        },
      ) => void;
    };
  };
};

export async function seedContextWindowTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Context Window Source Test",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected the seeded task to start a session");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  await testPage.evaluate((sessionId) => {
    const store = (window as ContextWindowStore).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    store.getState().setContextWindow(sessionId, {
      size: 258_400,
      used: 54_100,
      remaining: 204_300,
      efficiency: 21,
      source: "acp",
    });
  }, task.session_id);
}

export async function expectSourceRightOfTokenCount(contextTooltip: Locator): Promise<void> {
  const row = contextTooltip.getByTestId("context-window-token-row").first();
  const tokenCount = row.getByText("54.1K of 258.4K tokens");
  const source = row.getByText("ACP", { exact: true });
  const [tokenBox, sourceBox] = await Promise.all([tokenCount.boundingBox(), source.boundingBox()]);

  if (!tokenBox || !sourceBox) throw new Error("Expected token count and source to be rendered");
  if (sourceBox.x <= tokenBox.x + tokenBox.width) {
    throw new Error("Expected context source to render to the right of the token count");
  }
}
