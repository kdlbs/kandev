import {
  checkBuiltinCreation,
  checkBuiltinCreationBack,
  checkBuiltinCreationLoading,
} from "../../helpers/builtin-mcp-management";
import { expect, test } from "../../fixtures/test-base";
import {
  checkBuiltinPendingAddition,
  checkBuiltinPackageDetail,
  checkBuiltinUnavailableAcceptance,
  checkBuiltinDirtyFeedback,
} from "../../helpers/builtin-mcp-save-feedback";
import { checkBuiltinTask, checkBuiltinWorkspace } from "../../helpers/builtin-mcp-management";

test("saves built-in policy on a phone", async ({ testPage, apiClient }) => {
  await checkBuiltinWorkspace(testPage, apiClient, true);
});
test("opts in and prunes packages in the phone explorer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await checkBuiltinTask(testPage, apiClient, seedData, true);
});

test("records explicit Configuration consent at task creation", async ({ testPage, apiClient }) => {
  test.setTimeout(120_000);
  await checkBuiltinCreation(testPage, apiClient, true);
});

test("keeps built-in choices available while additional servers load on a phone", async ({
  testPage,
}) => {
  await checkBuiltinCreationLoading(testPage, true);
});

test("keeps translated detail usable across phone and touch widths", async ({
  testPage,
  apiClient,
}, testInfo) => {
  const workspace = await apiClient.createWorkspace(`Package layout ${Date.now()}`);
  try {
    await testPage.goto(`/settings/workspaces/${workspace.id}/mcp-servers`);
    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=pt-pt; path=/";
      localStorage.setItem("theme", "dark");
    });
    await testPage.reload();
    await testPage.getByTestId("builtin-mcp-package-configuration").tap();
    const detail = testPage.getByTestId("builtin-mcp-detail");
    for (const width of [320, 393, 767, 768, 900]) {
      await testPage.setViewportSize({ width, height: 852 });
      const choice = detail.getByTestId("builtin-mcp-choice-configuration");
      await choice.scrollIntoViewIfNeeded();
      await expect(choice).toBeVisible();
      expect((await choice.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      await testPage.screenshot({ path: testInfo.outputPath(`builtin-pt-dark-${width}.png`) });
    }
    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=en; path=/";
      localStorage.setItem("theme", "light");
    });
    await testPage.reload();
    await testPage.getByTestId("builtin-mcp-package-core").tap();
    await expect(detail.getByRole("combobox")).toHaveCount(0);
    await expect(detail).toContainText("required");
    await testPage.screenshot({ path: testInfo.outputPath("builtin-core-light-900.png") });
  } finally {
    await apiClient.deleteWorkspace(workspace.id, workspace.name);
  }
});

test("Back from package detail does not submit a new task", async ({ testPage, prCapture }) => {
  await checkBuiltinCreationBack(testPage, true, prCapture);
});

test("shows a saved package addition separately from current connections on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkBuiltinPendingAddition(testPage, apiClient, seedData, true);
});
test("shows change-aware package controls only for unsaved edits on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkBuiltinDirtyFeedback(testPage, apiClient, seedData, true);
});

test("identifies package details and hides empty exclusions", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkBuiltinPackageDetail(testPage, apiClient, seedData, true);
  await testPage
    .getByTestId("mcp-server-explorer")
    .screenshot({ path: testInfo.outputPath("package-configuration.png") });
});
test("keeps a saved package visible without current acceptance", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await checkBuiltinUnavailableAcceptance(testPage, apiClient, seedData, true);
  await testPage
    .getByTestId("mcp-server-explorer")
    .screenshot({ path: testInfo.outputPath("configuration-saved-unknown.png") });
});
