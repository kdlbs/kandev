import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { waitForStableActiveSession } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

export const SAME_TASK_QUEUED_MESSAGE = "SAME-TASK-QUEUED-FROM-ASTRA-6Q3V";
export const SAME_TASK_LONG_SENDER_NAME = "Astra reviewer session with extended details 4N8P";

export type SameTaskAttributionScenario = {
  taskId: string;
  title: string;
  senderSessionId: string;
  senderSessionName: string;
};

/**
 * Creates a live sender session, then a running sibling destination. The test
 * sends through the sender's mock-agent MCP server after both identities exist,
 * so the queue metadata and UI projection come from the real delivery path.
 */
export async function seedSameTaskAttributionScenario(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  senderSessionName = SAME_TASK_LONG_SENDER_NAME,
): Promise<SameTaskAttributionScenario> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: 'e2e:message("sender ready")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "same-task sender must finish its setup turn before sending to a sibling",
    timeout: 60_000,
  });
  await apiClient.renameSession(task.session_id, senderSessionName);

  return {
    taskId: task.id,
    title,
    senderSessionId: task.session_id,
    senderSessionName,
  };
}

export async function startSameTaskReceiverSession(
  page: Page,
  apiClient: ApiClient,
  scenario: SameTaskAttributionScenario,
): Promise<string> {
  const session = new SessionPage(page);
  const isTouch = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
  if (isTouch) {
    await page.getByTestId("mobile-sessions-pill").tap();
    await page.getByTestId("mobile-launch-session").tap();
  } else {
    await session.openNewSessionDialog();
  }

  await session.newSessionPromptInput().fill("/slow 45s");
  if (isTouch) await session.newSessionStartButton().tap();
  else await session.newSessionStartButton().click();
  await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

  let receiverSessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(scenario.taskId);
        receiverSessionId =
          sessions.find((candidate) => candidate.id !== scenario.senderSessionId)?.id ?? "";
        return receiverSessionId;
      },
      { timeout: 30_000, message: "new same-task receiver session should be created" },
    )
    .not.toBe("");
  await waitForSessionState(apiClient, {
    taskId: scenario.taskId,
    sessionId: receiverSessionId,
    expectedState: "RUNNING",
    message: "same-task receiver should remain running while its message is queued",
    timeout: 30_000,
  });
  return receiverSessionId;
}

export async function selectSameTaskSession(
  page: Page,
  session: SessionPage,
  sessionId: string,
): Promise<void> {
  const isTouch = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
  if (isTouch) {
    const sessionsPill = page.getByTestId("mobile-sessions-pill");
    if ((await sessionsPill.getAttribute("aria-expanded")) !== "true") {
      await sessionsPill.tap();
    }
    await page.getByTestId(`mobile-session-row-${sessionId}`).tap();
  } else {
    await session.sessionTabBySessionId(sessionId).click();
  }
  await waitForStableActiveSession(page, sessionId);
}
