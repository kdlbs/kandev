import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  exerciseManagedConversationChat,
  openManagedConversationChat,
} from "./managed-conversation-chat-helpers";

test.describe("Mobile plugin managed conversation chat", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await uninstallFixturePlugin(apiClient);
    await apiClient.updateWorkspace(seedData.workspaceId, { default_agent_profile_id: "" });
  });

  test("keeps the real managed chat flow usable on a phone", async ({
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

    const send = testPage.getByRole("button", { name: "Send" });
    const sendBox = await send.boundingBox();
    expect(sendBox).not.toBeNull();
    expect(sendBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await exerciseManagedConversationChat(testPage, prCapture, "mobile");
  });
});
