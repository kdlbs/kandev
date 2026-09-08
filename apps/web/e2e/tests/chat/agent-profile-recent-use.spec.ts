import { test, expect } from "../../fixtures/test-base";
import { openQuickChatSetup, startQuickChatFromSetup } from "./quick-chat-helpers";

test.describe("Agent profile recent use", () => {
  test("orders the last successful profile first, ignores cancellation, and survives reload", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((candidate) => candidate.id !== "dynamic") ?? agents[0];
    if (!agent) throw new Error("E2E fixture has no agent available for profile recency");

    const createdProfileIds: string[] = [];
    const profileA = await apiClient.createAgentProfile(agent.id, "Recent Use Profile A", {
      model: "mock-fast",
    });
    createdProfileIds.push(profileA.id);
    const profileB = await apiClient.createAgentProfile(agent.id, "Recent Use Profile B", {
      model: "mock-fast",
    });
    createdProfileIds.push(profileB.id);

    try {
      const dialog = await openQuickChatSetup(testPage);
      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.click();
      const listbox = testPage.getByRole("listbox");
      await listbox.getByRole("option", { name: profileB.name, exact: false }).click();

      await startQuickChatFromSetup(dialog, testPage);

      await expect
        .poll(async () => {
          const records = await apiClient.listAgentProfileRecentUse();
          return records.find((record) => record.context === "quick_chat")?.profile_ids[0] ?? "";
        })
        .toBe(profileB.id);

      await dialog.getByTestId("quick-chat-add-menu-trigger").click();
      await testPage.getByTestId("quick-chat-new-agent").click();
      const cancelledSetup = dialog;
      const cancelledSelector = cancelledSetup.getByTestId("agent-profile-selector");
      await cancelledSelector.click();
      await testPage
        .getByRole("listbox")
        .getByRole("option", { name: profileA.name, exact: false })
        .click();
      await cancelledSetup
        .getByTestId("quick-chat-setup-footer")
        .getByRole("button", { name: "Cancel", exact: true })
        .click();

      const reopenedSetup = await openQuickChatSetup(testPage, false);
      const reopenedSelector = reopenedSetup.getByTestId("agent-profile-selector");
      await reopenedSelector.click();
      await expect(testPage.getByRole("listbox").getByRole("option").first()).toContainText(
        profileB.name,
      );

      await testPage.keyboard.press("Escape");
      await testPage.reload({ waitUntil: "networkidle" });

      const afterReload = await openQuickChatSetup(testPage, false);
      const afterReloadSelector = afterReload.getByTestId("agent-profile-selector");
      await afterReloadSelector.click();
      await expect(testPage.getByRole("listbox").getByRole("option").first()).toContainText(
        profileB.name,
      );
    } finally {
      for (const profileId of createdProfileIds) {
        await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      }
    }
  });
});
