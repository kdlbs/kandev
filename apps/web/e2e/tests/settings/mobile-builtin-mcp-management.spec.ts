import { checkBuiltinCreation } from "../../helpers/builtin-mcp-management";
import { expect, test } from "../../fixtures/test-base";
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
