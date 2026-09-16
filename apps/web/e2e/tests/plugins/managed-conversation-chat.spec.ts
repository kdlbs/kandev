import { test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  exerciseManagedConversationChat,
  openManagedConversationChat,
} from "./managed-conversation-chat-helpers";

test.describe("Plugin managed conversation chat", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await uninstallFixturePlugin(apiClient);
    await apiClient.updateWorkspace(seedData.workspaceId, { default_agent_profile_id: "" });
  });

  test("loads, sends, streams, and surfaces a terminal descriptor error", async ({
    testPage,
    prCapture,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_agent_profile_id: seedData.agentProfileId,
    });
    await openManagedConversationChat(testPage);
    await exerciseManagedConversationChat(testPage, prCapture, "desktop");
  });
});
