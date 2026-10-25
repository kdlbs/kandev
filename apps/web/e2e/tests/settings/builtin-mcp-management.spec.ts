import { checkBuiltinCreation } from "../../helpers/builtin-mcp-management";
import { expect, test } from "../../fixtures/test-base";
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
    await expect(testPage.getByTestId("builtin-mcp-choice-diagnostics")).toContainText("Blocked");
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

test.afterEach(async ({ backend }, testInfo) => {
  if (testInfo.status !== testInfo.expectedStatus)
    await testInfo.attach("builtin-backend.log", {
      path: backend.logPath,
      contentType: "text/plain",
    });
});
