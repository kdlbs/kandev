import { type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";

type SeedOptions = {
  /** Mock-agent scenario slug, e.g. "clarification" or "simple-message". */
  scenario: string;
  /**
   * Wait for the chat to go idle after load. Leave false for blocking
   * clarification scenarios where the agent parks on the MCP call and the idle
   * input never appears until the question is answered.
   */
  waitForIdle?: boolean;
};

/**
 * Create a task + session running a mock-agent scenario, navigate to it, and
 * return a ready SessionPage. Shared by the clarification specs (overlay,
 * resize, and mobile) so the seed → navigate → waitForLoad flow lives in one
 * place.
 */
export async function seedClarificationSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  { scenario, waitForIdle = false }: SeedOptions,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  if (waitForIdle) await session.waitForChatIdle();

  return session;
}
