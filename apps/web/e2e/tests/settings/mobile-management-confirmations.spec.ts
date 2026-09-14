import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "../plugins/plugin-test-helpers";

test("phone plugin uninstall preserves the row and works from plugin details", async ({
  testPage,
  apiClient,
}) => {
  try {
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const row = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(row).toBeVisible();
    const trigger = row.getByRole("button", { name: "Uninstall", exact: true });
    await trigger.tap();
    const confirmation = testPage.getByRole("dialog", { name: "Uninstall plugin" });
    await expect(confirmation).toContainText("Kandev E2E Fixture Plugin");
    await expect(confirmation).toContainText("revoke its API key");
    await expect(testPage.getByRole("dialog")).toHaveCount(1);
    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(trigger).toBeFocused();
    await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
    const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
    await detail.getByRole("button", { name: "Uninstall", exact: true }).tap();
    const removed = testPage.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/plugins/${PLUGIN_ID}`) &&
        response.request().method() === "DELETE" &&
        response.ok(),
    );
    await testPage.getByTestId("plugin-uninstall-confirm").tap();
    await removed;
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);
    await expect(row).toHaveCount(0);
  } finally {
    await uninstallPluginFixture(apiClient);
  }
});

test("phone workflow removal preserves its form and retries a failed request in one dialog", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const endpoint = `/api/v1/workflow-sync/config?workspace_id=${seedData.workspaceId}`;
  const seeded = await apiClient.rawRequest("POST", endpoint, {
    provider: "github",
    repo_owner: "acme",
    repo_name: "workflows",
    project_path: "",
    branch: "main",
    path: ".kandev/workflows",
    poll_enabled: false,
    interval_seconds: 300,
  });
  expect(seeded.ok).toBe(true);
  try {
    await new WorkflowSettingsPage(testPage).goto(seedData.workspaceId);
    await testPage.getByTestId("workflow-sync-open").tap();
    const dialog = testPage.getByTestId("workflow-sync-dialog");
    const dialogId = await dialog.getAttribute("id");
    const branch = dialog.getByTestId("workflow-sync-branch-input");
    await branch.fill("draft-branch");
    const trigger = dialog.getByTestId("workflow-sync-remove");
    await trigger.tap();
    await expect(testPage.getByRole("dialog")).toHaveCount(1);
    await expect(dialog).toHaveAttribute("id", dialogId!);
    await expect(branch).toBeHidden();
    await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();
    await dialog.getByRole("button", { name: "Back" }).tap();
    await expect(branch).toHaveValue("draft-branch");
    await expect(trigger).toBeFocused();
    let attempts = 0;
    await testPage.route("**/api/v1/workflow-sync/config?*", async (route) => {
      if (route.request().method() !== "DELETE") return route.continue();
      attempts += 1;
      if (attempts !== 1) return route.continue();
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Temporary removal failure" }),
      });
    });
    await trigger.tap();
    await waitForFiniteAnimations(dialog);
    const cancel = dialog.getByRole("button", { name: "Cancel" });
    const cancelBox = await cancel.boundingBox();
    expect(cancelBox).not.toBeNull();
    expect(cancelBox!.height).toBeGreaterThanOrEqual(48);
    expect(cancelBox!.y + cancelBox!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
    await prCapture.screenshot("workflow-sync-removal-step", {
      caption: "Workflow sync removal is a focused step in the same phone form dialog.",
    });
    const confirmation = dialog.getByTestId("workflow-sync-remove-confirm");
    await confirmation.tap();
    await expect.poll(() => attempts).toBe(1);
    await expect(confirmation).toBeEnabled();
    await expect(dialog).toBeVisible();
    await expect(branch).toHaveValue("draft-branch");
    const removed = testPage.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/workflow-sync/config") &&
        response.request().method() === "DELETE" &&
        response.ok(),
    );
    await confirmation.tap();
    await removed;
    await expect(dialog).toBeHidden();
    expect(attempts).toBe(2);
    const persisted = await apiClient.rawRequest("GET", endpoint);
    expect(persisted.status).toBe(204);
  } finally {
    await testPage.unroute("**/api/v1/workflow-sync/config?*");
    await apiClient.rawRequest("DELETE", endpoint);
  }
});
