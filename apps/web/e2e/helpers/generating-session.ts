import { type Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { waitForActiveSessionForegroundActivity } from "./session-store";
import { SessionPage } from "../pages/session-page";

export interface SeedRunningGeneratingSessionOptions {
  // Duration for the default `/sleep N` predecessor. Ignored when
  // predecessorPrompt is set.
  sleepSeconds?: number;
  // Overrides the default `/sleep N` predecessor entirely — used by the
  // folded/deferred tests to seed steer-fold-setup / steer-defer-setup
  // instead.
  predecessorPrompt?: string;
}

/**
 * Seeds a session whose foreground turn is still generating, which is the state
 * that normally queues further input. A no-tool `/sleep` holds the turn open
 * without emitting output, so a test can send into a genuinely busy session
 * without racing agent activity.
 */
export async function seedRunningGeneratingSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  options: SeedRunningGeneratingSessionOptions = {},
): Promise<{ session: SessionPage; taskId: string; sessionId: string }> {
  const { sleepSeconds = 60, predecessorPrompt } = options;
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
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessage(predecessorPrompt ?? `/sleep ${sleepSeconds}`);
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForActiveSessionForegroundActivity(testPage, "generating");
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  return { session, taskId: task.id, sessionId: task.session_id };
}
