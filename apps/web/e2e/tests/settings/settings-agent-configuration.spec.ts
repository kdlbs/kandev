import { test, expect } from "../../fixtures/test-base";
import {
  expandSettingsToolCalls,
  startSettingsSession,
  waitForSettingsSession,
} from "./settings-agent-configuration-helpers";

test.describe("Agent-accessible settings", () => {
  test("searches and describes settings through the real MCP transport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
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
      await expect((await apiClient.getAgentProfile(seedData.agentProfileId)).name).toBe(
        "Agent settings E2E",
      );
    } finally {
      await apiClient.updateAgentProfile(seedData.agentProfileId, { name: originalProfile.name });
    }
  });
});
