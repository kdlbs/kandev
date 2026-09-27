import { type Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { waitForSessionDone } from "./session";
import {
  waitForActiveSessionForegroundActivity,
  waitForSessionAgentctlReady,
} from "./session-store";
import { SessionPage } from "../pages/session-page";

export interface SeedRunningGeneratingSessionOptions {
  // Duration for the default `/sleep N` predecessor. Ignored when
  // predecessorPrompt is set.
  sleepSeconds?: number;
  // Overrides the default `/sleep N` predecessor entirely — used by the
  // folded/deferred tests to seed steer-fold-setup / steer-defer-setup
  // instead.
  predecessorPrompt?: string;
  // Starts the task directly with the generating turn instead of waiting for
  // the setup-only simple-message turn to complete first.
  startWithGeneratingTurn?: boolean;
  // Lets a caller stop the newly created session if a later setup step fails.
  onSessionCreated?: (identity: { taskId: string; sessionId: string }) => void;
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
  const { sleepSeconds = 60, predecessorPrompt, startWithGeneratingTurn = false } = options;
  const generatingPrompt = predecessorPrompt ?? `/sleep ${sleepSeconds}`;
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: startWithGeneratingTurn ? generatingPrompt : "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  options.onSessionCreated?.({ taskId: task.id, sessionId: task.session_id });
  if (!startWithGeneratingTurn) {
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "the initial task prompt should finish before seeding a generating turn",
      90_000,
    );
  }
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await waitForSessionAgentctlReady(testPage, task.session_id);
  if (!startWithGeneratingTurn) {
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.sendMessage(generatingPrompt);
  }
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForActiveSessionForegroundActivity(testPage, "generating");
  return { session, taskId: task.id, sessionId: task.session_id };
}
