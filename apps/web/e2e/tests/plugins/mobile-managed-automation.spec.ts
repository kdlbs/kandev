import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  ensureManagedConversation,
  installAndGrantManagedConversationAccess,
  seedManagedAutomation,
} from "./managed-automation-helpers";

test.describe("managed conversation automation on phone", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("phone destination editor", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    await installAndGrantManagedConversationAccess(testPage, apiClient, seedData.workspaceId);
    const destination = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "phone-e2e-managed",
      seedData.agentProfileId,
    );
    const automation = await seedManagedAutomation(
      apiClient,
      seedData.workspaceId,
      "Phone managed schedule",
      destination,
    );

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    const editor = testPage.getByTestId("automation-editor");
    await expect(editor).toBeVisible();
    await expect(
      testPage.getByRole("radio", { name: "Managed conversation", exact: false }),
    ).toBeChecked();

    const trigger = testPage.getByTestId("managed-conversation-destination-trigger");
    await expect(trigger).toContainText("phone-e2e-managed");
    await expect
      .poll(async () => (await trigger.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const drawer = testPage.getByTestId("managed-conversation-destination-drawer");
    await expect(drawer).toBeVisible();
    const option = drawer.getByText("Kandev E2E Fixture Plugin / phone-e2e-managed", {
      exact: true,
    });
    await expect(option).toBeVisible();
    await option.tap();
    await expect(drawer).toBeHidden();
    await expect(trigger).toContainText("phone-e2e-managed");
  });
});
