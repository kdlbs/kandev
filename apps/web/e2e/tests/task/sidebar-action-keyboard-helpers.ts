import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export async function seedKeyboardRows(apiClient: ApiClient, seedData: SeedData) {
  const options = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const navigation = await apiClient.seedTask(
    seedData.workspaceId,
    "Keyboard navigation task",
    options,
  );
  const target = await apiClient.seedTask(seedData.workspaceId, "Keyboard action target", options);
  for (const task of [navigation, target]) {
    await apiClient.seedTaskSession(task.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
  }
  return { navigation, target };
}

export async function tabTo(page: Page, target: Locator) {
  for (let attempt = 0; attempt < 150; attempt++) {
    if (await target.evaluate((element) => document.activeElement === element)) return;
    await page.keyboard.press("Tab");
  }
  await expect(target).toBeFocused();
}
