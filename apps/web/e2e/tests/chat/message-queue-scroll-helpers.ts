import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const FULL_QUEUE_SIZE = 10;

export async function seedFullQueueTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Queue scrolling regression",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
  });

  for (let index = 0; index < FULL_QUEUE_SIZE; index++) {
    await apiClient.queueMessage(
      task.id,
      sessionId,
      `Queued item ${index + 1}\nSecond line keeps every queue row realistically tall.`,
    );
  }

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

export async function expectFullQueueScrolls(session: SessionPage): Promise<void> {
  const chat = session.activeChat();
  const chip = chat.getByTestId("queue-chip");
  await expect(chip).toBeVisible({ timeout: 10_000 });
  await chip.click();

  const panel = chat.getByTestId("queued-ghost-list");
  const entries = panel.getByTestId("queue-entry-text");
  await expect(entries).toHaveCount(FULL_QUEUE_SIZE);

  const scrollRegion = panel.getByTestId("queue-scroll-region");
  await expect(scrollRegion).toBeVisible({ timeout: 5_000 });
  const geometry = await scrollRegion.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      overflowY: style.overflowY,
    };
  });
  expect(geometry.scrollHeight).toBeGreaterThan(geometry.clientHeight);
  expect(geometry.overflowY).toBe("auto");

  const composer = chat.getByTestId("chat-input-editor-shell");
  const [chatBox, composerBox] = await Promise.all([chat.boundingBox(), composer.boundingBox()]);
  expect(chatBox).not.toBeNull();
  expect(composerBox).not.toBeNull();
  expect(composerBox!.y + composerBox!.height).toBeLessThanOrEqual(
    chatBox!.y + chatBox!.height + 1,
  );

  await scrollRegion.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect(entries.last()).toBeInViewport();
}
