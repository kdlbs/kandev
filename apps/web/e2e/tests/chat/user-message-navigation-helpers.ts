import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export const OLD_USER_PROMPT = "OLD-USER-PROMPT-8N4K";
export const CURRENT_USER_PROMPT = "CURRENT-USER-PROMPT-6M2R";
export const FILLER_COUNT = 160;

export type LongHistorySeed = {
  taskId: string;
  sessionId: string;
};

export async function seedLongUserMessageHistory(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<LongHistorySeed> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "user-message-navigation",
    seedData.agentProfileId,
    {
      description: `e2e:message("initial turn complete")\n# ${OLD_USER_PROMPT}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id!;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => message.content.includes("initial turn complete"));
      },
      { timeout: 60_000, message: "Waiting for the initial turn to persist" },
    )
    .toBe(true);

  await apiClient.seedAgentMessages(sessionId, FILLER_COUNT, "navigation filler");
  await apiClient.addUserMessage(
    task.id,
    sessionId,
    `e2e:message("current turn complete")\n# ${CURRENT_USER_PROMPT}`,
  );
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) =>
            message.author_type === "user" && message.content.includes(CURRENT_USER_PROMPT),
        );
      },
      { timeout: 30_000, message: "Waiting for the current user prompt to persist" },
    )
    .toBe(true);

  return { taskId: task.id, sessionId };
}

export async function openLongHistory(
  page: Page,
  session: SessionPage,
  taskId: string,
  renderer: "native" | "virtuoso" = "native",
) {
  await page.goto(`/t/${taskId}?renderer=${renderer}`);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect(session.userMessageContaining(CURRENT_USER_PROMPT)).toBeVisible({ timeout: 30_000 });
  await expect(session.userMessageContaining(OLD_USER_PROMPT)).toHaveCount(0);
}

export async function expectMessageAtNavigationPosition(scrollOwner: Locator, message: Locator) {
  await expect
    .poll(
      async () => {
        const [scrollBox, messageBox, scrollState] = await Promise.all([
          scrollOwner.boundingBox(),
          message.boundingBox(),
          scrollOwner.evaluate((element) => ({
            scrollTop: element.scrollTop,
            maxScrollTop: element.scrollHeight - element.clientHeight,
          })),
        ]);
        if (!scrollBox || !messageBox) return false;
        const scrollCenter = scrollBox.y + scrollBox.height / 2;
        const messageCenter = messageBox.y + messageBox.height / 2;
        const isFullyVisible =
          messageBox.y >= scrollBox.y - 1 &&
          messageBox.y + messageBox.height <= scrollBox.y + scrollBox.height + 1;
        const isCentered = Math.abs(scrollCenter - messageCenter) <= 40;
        const isAtTopBoundary = scrollState.scrollTop <= 1 && messageCenter <= scrollCenter;
        const isAtBottomBoundary =
          scrollState.maxScrollTop - scrollState.scrollTop <= 1 && messageCenter >= scrollCenter;
        return isFullyVisible && (isCentered || isAtTopBoundary || isAtBottomBoundary);
      },
      {
        timeout: 10_000,
        message: "Waiting for the user prompt to center or reach the scroll boundary",
      },
    )
    .toBe(true);
}

export async function expectNavigationOutline(message: Locator) {
  await expect(message).toHaveCSS("outline-style", "solid");
  await expect(message).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
}
