import { Buffer } from "node:buffer";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { AutomationsPage } from "../../pages/automations-page";
import {
  ensureManagedConversation,
  installAndGrantManagedConversationAccess,
  listManagedDestinations,
  seedManagedAutomation,
} from "./managed-automation-helpers";

async function readWorkspaceAutomationExport(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/automations/export`,
  );
  if (!response.ok) throw new Error(`Could not export automations: ${response.status}`);
  return response.text();
}

test.describe("managed conversation automations", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("schedule delivery, cleanup and portable rebinding", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installAndGrantManagedConversationAccess(testPage, apiClient, seedData.workspaceId);
    const source = await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "exported-e2e-managed",
      seedData.agentProfileId,
    );
    await ensureManagedConversation(
      apiClient,
      seedData.workspaceId,
      "receiving-e2e-managed",
      seedData.agentProfileId,
    );
    const original = await seedManagedAutomation(
      apiClient,
      seedData.workspaceId,
      "Portable managed schedule",
      source,
    );
    const tasksBefore = await apiClient.listTasks(seedData.workspaceId);

    const trigger = await apiClient.triggerAutomationManual(original.id);
    expect(trigger.skipped).not.toBe(true);
    expect(trigger.run_id).toBeTruthy();
    expect(trigger.delivery_status).toBeTruthy();
    await expect
      .poll(async () => (await apiClient.listTasks(seedData.workspaceId)).tasks.length)
      .toBe(tasksBefore.tasks.length);

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await automations.openByName("Portable managed schedule");
    await expect(testPage.getByTestId("managed-conversation-destination-trigger")).toContainText(
      "exported-e2e-managed",
    );
    await testPage.getByText(/Recent Runs?\s*\(/).click();
    const runRow = testPage.getByTestId(`run-row-${trigger.run_id}`);
    await expect(runRow).toBeVisible();
    await expect(runRow.getByTestId("run-delivery-status")).toBeVisible();

    const exported = await readWorkspaceAutomationExport(apiClient, seedData.workspaceId);
    expect(exported).toContain("plugin_id: kandev-plugin-e2e");
    expect(exported).toContain("instance_key: exported-e2e-managed");
    expect(exported).not.toMatch(/installation_id|conversation_id|session_id|receipt_id/);

    await automations.deleteButton.click();
    await automations.deleteConfirmButton.click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });
    const remainingAfterCleanup = await listManagedDestinations(apiClient, seedData.workspaceId);
    expect(remainingAfterCleanup.destinations.map((item) => item.instance_key)).toEqual(
      expect.arrayContaining(["exported-e2e-managed", "receiving-e2e-managed"]),
    );

    await testPage.getByTestId("import-managed-automation-button").click();
    const importFile = testPage.getByTestId("managed-automation-import-file");
    await importFile.setInputFiles({
      name: "kandev-automations.yaml",
      mimeType: "application/yaml",
      buffer: Buffer.from(exported),
    });
    const importEntry = testPage.getByTestId("managed-automation-import-entry-0");
    const importSubmit = importEntry.getByTestId("managed-automation-import-submit-0");
    await expect(importSubmit).toBeDisabled();
    await importEntry.getByTestId("managed-conversation-destination-trigger").click();
    await testPage.getByText("Kandev E2E Fixture Plugin / receiving-e2e-managed").click();
    await expect(importSubmit).toBeEnabled();
    await importSubmit.click();
    await expect(importEntry.getByText("Imported", { exact: true })).toBeVisible();

    await automations.goto();
    await automations.openByName("Portable managed schedule");
    await expect(testPage.getByTestId("managed-conversation-destination-trigger")).toContainText(
      "receiving-e2e-managed",
    );
    await automations.deleteButton.click();
    await automations.deleteConfirmButton.click();
    const remainingAfterImportCleanup = await listManagedDestinations(
      apiClient,
      seedData.workspaceId,
    );
    expect(remainingAfterImportCleanup.destinations.map((item) => item.instance_key)).toEqual(
      expect.arrayContaining(["exported-e2e-managed", "receiving-e2e-managed"]),
    );
  });
});
