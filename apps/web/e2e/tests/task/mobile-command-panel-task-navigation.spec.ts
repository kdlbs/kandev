import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile: command panel task navigation", () => {
  test("navigates directly without targeting the hidden desktop sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const source = await apiClient.seedTask(seedData.workspaceId, "Mobile command source task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const destination = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile command destination task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await testPage.goto(`/t/${source.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage.getByTestId("mobile-session-menu")).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar-layout")).toBeHidden();

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.getByRole("combobox").fill("Mobile command destination");
    const option = dialog
      .getByRole("option")
      .filter({ hasText: "Mobile command destination task" })
      .first();
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${destination.task_id}$`));
    const mobileTopBar = testPage
      .getByTestId("mobile-session-menu")
      .locator("xpath=ancestor::header");
    await expect(
      mobileTopBar.getByText("Mobile command destination task", { exact: true }),
    ).toBeVisible();
    await expect(testPage.getByTestId("app-sidebar-layout")).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
