import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

const PLUGIN_ID = "kandev-plugin-bitbucket";
const packagePath = process.env.KANDEV_BITBUCKET_PLUGIN_PACKAGE?.trim();

test.skip(!packagePath, "requires KANDEV_BITBUCKET_PLUGIN_PACKAGE from the attached plugin repo");

async function installPackagedPlugin(testPage: import("@playwright/test").Page): Promise<void> {
  if (!packagePath) throw new Error("Bitbucket plugin package path is required");
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
): Promise<unknown> {
  const response = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/actions/${key}`, {
    workspaceId,
  });
  const body = await response.text();
  expect(response.status, body).toBe(200);
  return JSON.parse(body) as unknown;
}

test.describe("Bitbucket packaged plugin", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("installs the real package through its unconfigured desktop lifecycle", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "desktop" });

    await installPackagedPlugin(testPage);

    // These are real RPC calls into the uploaded artifact, not fixture-provider
    // coverage. An empty connection must stay safe and useful without requiring
    // a live Bitbucket account or test credential.
    await expect(
      invokePluginAction(apiClient, "connection.get", seedData.workspaceId),
    ).resolves.toEqual({
      state: "unconfigured",
      healthy: false,
    });
    await expect(
      invokePluginAction(apiClient, "repositories.list", seedData.workspaceId),
    ).resolves.toEqual({
      repositories: [],
    });
    await expect(
      invokePluginAction(apiClient, "pullrequests.queue", seedData.workspaceId),
    ).resolves.toEqual({
      pull_requests: [],
    });

    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toBeVisible();
    await expect(testPage.getByTestId("bitbucket-connection-health")).toBeVisible();
    await expect(testPage.getByText("Not configured", { exact: true })).toBeVisible();
    await expect(
      testPage.getByText("No repositories available for this connection."),
    ).toBeVisible();
    await expect(testPage.getByText("No pull requests", { exact: true })).toBeVisible();
    await capture.screenshot("desktop-bitbucket-workbench", {
      caption: "Desktop Bitbucket workbench from the packaged plugin",
    });

    // Deactivation must revoke the actual package's navigation/runtime entries;
    // enabling it loads the uploaded package again, not the fixture contract.
    await testPage.goto("/settings/plugins");
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible();
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toHaveCount(0);
    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toBeVisible();
    capture.flush();
  });

  test("renders the uploaded package's mobile workbench in a touch-sized viewport", async ({
    testPage,
  }, testInfo) => {
    test.setTimeout(90_000);
    const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: "mobile" });

    await installPackagedPlugin(testPage);

    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage.goto("/bitbucket");
    await expect(testPage.getByTestId("bitbucket-workbench")).toHaveClass(/bb-mobile/);
    const filterButton = testPage.getByRole("button", { name: "Open Bitbucket filters" });
    await expect(filterButton).toBeVisible();
    expect((await filterButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    // This spec runs in the regular packaged-plugin project; resizing its
    // isolated context selects the phone composition, while the dedicated
    // fixture-contract spec exercises Playwright's touch project.
    await filterButton.click();
    await expect(testPage.getByRole("heading", { name: "Queue filters" })).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
    await capture.screenshot("mobile-bitbucket-filters", {
      caption: "Mobile Bitbucket workbench with its touch filter drawer",
    });
    capture.flush();
  });
});
