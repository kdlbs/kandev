import { type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForLatestSessionDone } from "../../helpers/session";
import { waitForSessionAgentctlReady } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";
import { multiMessageScript, planScript } from "../../helpers/seed-session-messages";

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";
export { MODIFIER };

/** Create a task+session seeded with a mock-agent script, navigate to it, wait for idle. */
export async function seedTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  opts: { description: string },
): Promise<{ session: SessionPage; taskId: string; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: opts.description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await waitForLatestSessionDone(
    apiClient,
    task.id,
    1,
    `wait for seeded search session ${task.id} to finish its initial turn`,
    60_000,
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 60_000 });
  if (!task.session_id) throw new Error("seedTask did not return a session_id");
  await waitForSessionAgentctlReady(page, task.session_id);
  return { session, taskId: task.id, sessionId: task.session_id! };
}

/** Seed N agent messages with distinct content in a single turn. */
export function seedMessagesDescription(lines: string[]): string {
  return multiMessageScript(lines, 5);
}

/** Build a plan-seeding description. */
export { planScript };
