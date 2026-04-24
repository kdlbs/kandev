import { type Page, expect } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
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
  opts: { description: string; rendererOverride?: "native" | "virtuoso" },
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
  const query = opts.rendererOverride ? `?renderer=${opts.rendererOverride}` : "";
  await page.goto(`/t/${task.id}${query}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return { session, taskId: task.id, sessionId: task.session_id! };
}

/** Seed N agent messages with distinct content in a single turn. */
export function seedMessagesDescription(lines: string[]): string {
  return multiMessageScript(lines, 5);
}

/** Build a plan-seeding description. */
export { planScript };
