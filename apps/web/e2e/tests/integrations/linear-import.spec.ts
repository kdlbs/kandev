import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Linear import bar", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.setLinearConfig({
      secret: "lin_api_xxx",
    });
    await apiClient.waitForIntegrationAuthHealthy("linear");
  });

  test("pasting a known identifier fills the title and description", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockLinearAddIssues([
      {
        id: "issue-1",
        identifier: "ENG-12",
        title: "Add billing endpoint",
        description: "Spec lives at notion://...",
        url: "https://linear.app/mock-org/issue/ENG-12",
      },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("linear-import-trigger").click();
    await testPage.getByTestId("linear-import-input").fill("ENG-12");
    await testPage.getByTestId("linear-import-submit").click();

    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      "[ENG-12] Add billing endpoint",
    );
    await expect(testPage.getByTestId("task-description-input")).toContainText(
      "Spec lives at notion://",
    );
  });

  test("invalid input shows the validation hint", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("linear-import-trigger").click();

    const submit = testPage.getByTestId("linear-import-submit");
    await expect(submit).toBeDisabled();

    await testPage.getByTestId("linear-import-input").fill("garbage");
    await submit.click();
    await expect(testPage.getByTestId("linear-import-error")).toContainText(
      /Paste a Linear issue URL or identifier/i,
    );
  });

  test("unknown identifier surfaces the backend error inline", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("linear-import-trigger").click();

    await testPage.getByTestId("linear-import-input").fill("NOPE-99");
    await testPage.getByTestId("linear-import-submit").click();
    await expect(testPage.getByTestId("linear-import-error")).toContainText(/NOPE-99/);
  });
});
