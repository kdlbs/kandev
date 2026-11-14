import {
  checkBuiltinCreation,
  checkBuiltinCreationBack,
  checkBuiltinCreationLoading,
} from "../../helpers/builtin-mcp-management";
import { expect, test } from "../../fixtures/test-base";
import {
  checkBuiltinInheritance,
  checkBuiltinPendingAddition,
  checkBuiltinPackageDetail,
  checkBuiltinUnavailableAcceptance,
  checkBuiltinDirtyFeedback,
} from "../../helpers/builtin-mcp-save-feedback";
import {
  activate,
  builtinView,
  checkBuiltinTask,
  checkBuiltinWorkspace,
  chooseBuiltin,
} from "../../helpers/builtin-mcp-management";

test("saves built-in workspace policy through the shared save bar", async ({
  testPage,
  apiClient,
}) => {
  await checkBuiltinWorkspace(testPage, apiClient, false);
});
test("opts a task into Configuration and prunes Walkthroughs", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await checkBuiltinTask(testPage, apiClient, seedData, false);
});
test("retains a denied draft and explains the failure", async ({ testPage, apiClient }) => {
  const workspace = await apiClient.createWorkspace(`Denied packages ${Date.now()}`);
  const path = `**/workspaces/${workspace.id}/mcp-built-in/policy`;
  try {
    await testPage.goto(`/settings/workspaces/${workspace.id}/mcp-servers`);
    await activate(testPage.getByTestId("builtin-mcp-package-diagnostics"), false);
    await chooseBuiltin(testPage, "diagnostics", "Blocked", false);
    await testPage.route(path, (route) =>
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify({ error_code: "builtin_mcp_forbidden" }),
      }),
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect(testPage.getByTestId("builtin-mcp-detail").getByRole("alert")).toContainText(
      "Your draft was not saved",
    );
    await expect(testPage.getByTestId("builtin-mcp-choice-diagnostics")).toContainText("Off");
    expect(
      (await builtinView(apiClient, "workspaces", workspace.id)).policy.packages.diagnostics,
    ).toBe("default");
  } finally {
    await testPage.unroute(path);
    await apiClient.deleteWorkspace(workspace.id, workspace.name);
  }
});

test("records explicit Configuration consent at task creation", async ({ testPage, apiClient }) => {
  test.setTimeout(120_000);
  await checkBuiltinCreation(testPage, apiClient, false);
});

test("keeps built-in choices available while additional servers load", async ({ testPage }) => {
  await checkBuiltinCreationLoading(testPage, false);
});

test.afterEach(async ({ backend }, testInfo) => {
  if (testInfo.status !== testInfo.expectedStatus)
    await testInfo.attach("builtin-backend.log", {
      path: backend.logPath,
      contentType: "text/plain",
    });
});

test("Back from package detail does not submit a new task", async ({ testPage, prCapture }) => {
  await checkBuiltinCreationBack(testPage, false, prCapture);
});

test("shows a saved package addition separately from current connections", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkBuiltinPendingAddition(testPage, apiClient, seedData, false);
});
test("shows change-aware package controls only for unsaved edits", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkBuiltinDirtyFeedback(testPage, apiClient, seedData, false);
});

test("identifies package details and hides empty exclusions", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkBuiltinPackageDetail(testPage, apiClient, seedData, false);
  await testPage
    .getByTestId("mcp-server-explorer")
    .screenshot({ path: testInfo.outputPath("package-configuration.png") });
});
test("keeps a saved package visible without current acceptance", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkBuiltinUnavailableAcceptance(testPage, apiClient, seedData, false);
  await testPage
    .getByTestId("mcp-server-explorer")
    .screenshot({ path: testInfo.outputPath("configuration-saved-unknown.png") });
});

// @covers AC-AGENTS-BUILTIN-MCP-005.2
test("restores inheritance when the effective switch value stays on", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkBuiltinInheritance(testPage, apiClient, seedData, false);
});
