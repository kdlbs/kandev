import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Real, packaged kandev-plugin-redmine contract coverage — installs the
// actual artifact built by yattdev/kandev-plugin-redmine's own `make
// package-host` (plan task 09), not a fixture provider. Scoped to what is
// genuinely verifiable without a live Redmine instance: the unconfigured
// lifecycle of every read-only, zero-network action, and the native UI
// wiring (settings page, shared Link dialog, disable/enable). Actions that
// require a real outbound HTTPS call to a Redmine base URL (connection.save,
// link.set against an actual issue) are exercised in the plugin repo's own
// test suite against a fake HTTP server, not here — see
// docs/plans/redmine-plugin/task-03..06-*.md and the "Not automated" note in
// docs/specs/redmine-plugin/spec.md.
const PLUGIN_ID = "kandev-plugin-redmine";
const packagePath = process.env.KANDEV_REDMINE_PLUGIN_PACKAGE?.trim();

test.skip(!packagePath, "requires KANDEV_REDMINE_PLUGIN_PACKAGE from the attached plugin repo");

async function installPackagedPlugin(testPage: Page): Promise<void> {
  if (!packagePath) throw new Error("Redmine plugin package path is required");
  await testPage.goto("/settings/plugins");
  await testPage.getByTestId("install-plugin-trigger").click();
  await testPage.getByTestId("install-plugin-tab-upload").click();
  await testPage.getByTestId("install-plugin-file-input").setInputFiles(path.resolve(packagePath));
  await testPage.getByTestId("install-plugin-upload-submit").click();
  const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
  await expect(pluginRow).toBeVisible({ timeout: 15_000 });
  await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
}

async function invokePluginAction(
  apiClient: import("../../helpers/api-client").ApiClient,
  key: string,
  workspaceId: string,
): Promise<{ status: number; body: unknown }> {
  const response = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/${key}`, {
    workspaceId,
  });
  const text = await response.text();
  return { status: response.status, body: text ? JSON.parse(text) : undefined };
}

test.describe("Redmine packaged plugin", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("installs the real package and exposes safe unconfigured defaults", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await installPackagedPlugin(testPage);

    // Real RPC calls into the uploaded artifact. Both are zero-network reads
    // (connection.get reads plugin_state; watches.list reads the plugin's
    // own watch store) so they are safe to assert deterministically without
    // a live Redmine target.
    const connection = await invokePluginAction(apiClient, "connection.get", seedData.workspaceId);
    expect(connection.status).toBe(200);
    expect(connection.body).toEqual({ state: "disconnected" });

    const watches = await invokePluginAction(apiClient, "watches.list", seedData.workspaceId);
    expect(watches.status).toBe(200);
    expect(watches.body).toEqual({ watches: [] });

    // projects.list needs a live Redmine client to call ListLive, so with no
    // connection configured it returns the plugin's classified-error shape
    // (still HTTP 200 — action errors are body-encoded, not host-level
    // errors) rather than an empty list. Assert the shape, not the exact
    // wording, since the underlying message is an implementation detail of
    // the plugin's connection service.
    const projects = await invokePluginAction(apiClient, "projects.list", seedData.workspaceId);
    expect(projects.status).toBe(200);
    expect(projects.body).toMatchObject({
      error: expect.any(String),
      kind: expect.any(String),
    });
  });

  test("renders the native settings page and the shared Link dialog, and revokes/restores them on disable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await installPackagedPlugin(testPage);

    // registerIntegrationSettings({id:"redmine",...}) renders natively at
    // /settings/integrations/redmine — the same route native (non-plugin)
    // integrations use, per src/integration-settings-route.tsx's fallthrough
    // to renderPluginIntegrationSettings.
    await testPage.goto("/settings/integrations/redmine");
    await expect(testPage.getByTestId("redmine-base-url-input")).toBeVisible();
    await expect(testPage.getByTestId("redmine-api-key-input")).toBeVisible();
    const saveButton = testPage.getByTestId("redmine-connection-save");
    await expect(saveButton).toBeDisabled();
    await expect(testPage.locator("#redmine-connection-state")).toHaveText("disconnected");

    await testPage.getByTestId("redmine-base-url-input").fill("https://redmine.example.com");
    await expect(saveButton).toBeDisabled();
    await testPage.getByTestId("redmine-api-key-input").fill("api-key-value");
    await expect(saveButton).toBeEnabled();

    // Native Link submenu action opens the shared host.openTaskLinkDialog,
    // not a plugin-drawn modal — verify the dialog contract without
    // submitting (submission calls link.set, which needs a real Redmine
    // target to resolve the issue reference against).
    const task = await apiClient.createTask(seedData.workspaceId, "Redmine plugin contract task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskContextMenu(task.id);
    const linkSubmenu = testPage.getByTestId("task-context-link");
    await linkSubmenu.focus();
    await testPage.keyboard.press("ArrowRight");
    const redmineLink = testPage.getByTestId(`task-context-link-plugin-${PLUGIN_ID}:redmine-link`);
    await expect(redmineLink).toHaveText("Redmine Issue");
    await redmineLink.click();
    const linkDialog = testPage.getByRole("dialog", { name: "Link Redmine issue" });
    await expect(linkDialog).toContainText('Enter a Redmine issue ID, "#123", or an issue URL.');
    await expect(linkDialog.getByTestId("redmine-link-input")).toBeVisible();
    await expect(linkDialog.getByTestId("redmine-link-submit")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(linkDialog).toBeHidden();

    // Disable must revoke the native settings registration and the Link
    // submenu entry; re-enable must restore both from the installed
    // package.
    await testPage.goto("/settings/plugins");
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible();
    await testPage.goto("/settings/integrations/redmine");
    await expect(testPage.getByTestId("redmine-base-url-input")).toHaveCount(0);

    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await testPage.goto("/settings/integrations/redmine");
    await expect(testPage.getByTestId("redmine-base-url-input")).toBeVisible();
  });
});
