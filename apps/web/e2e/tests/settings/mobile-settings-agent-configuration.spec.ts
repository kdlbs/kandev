import { test, expect } from "../../fixtures/test-base";
import {
  expandSettingsToolCalls,
  startSettingsSession,
  waitForSettingsSession,
} from "./settings-agent-configuration-helpers";

test.describe("Mobile agent-accessible settings", () => {
  test("keeps the compact settings flow visible on a phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const originalProfile = await apiClient.getAgentProfile(seedData.agentProfileId);
    try {
      const session = await startSettingsSession(apiClient, seedData);
      const page = await waitForSettingsSession(testPage, apiClient, session.task_id);

      await expandSettingsToolCalls(page);
      await expect(page.chat.getByText("Kandev: Search Settings")).toBeVisible();
      await expect(page.chat.getByText("Kandev: Describe Setting")).toBeVisible();
      await expect(page.chat.getByText("Kandev: List Settings Resources")).toBeVisible();
      await expect(page.chat.getByText("Kandev: Get Settings")).toBeVisible();
      await expect(page.chat.getByText("Kandev: Update Settings")).toBeVisible();
      expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
        await testPage.evaluate(() => document.documentElement.clientWidth),
      );
    } finally {
      await apiClient.updateAgentProfile(seedData.agentProfileId, { name: originalProfile.name });
    }
  });
});
