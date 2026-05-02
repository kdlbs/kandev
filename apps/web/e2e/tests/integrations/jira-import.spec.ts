import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Jira import bar", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");
  });

  test("pasting a known ticket key fills the title and description", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockJiraAddTickets([
      {
        key: "PROJ-12",
        summary: "Fix login redirect",
        description: "Body text from Jira",
        url: "https://acme.atlassian.net/browse/PROJ-12",
      },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("jira-import-trigger").click();
    await testPage.getByTestId("jira-import-input").fill("PROJ-12");
    await testPage.getByTestId("jira-import-submit").click();

    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      "[PROJ-12] Fix login redirect",
    );
    await expect(testPage.getByTestId("task-description-input")).toContainText(
      "Body text from Jira",
    );
  });

  test("invalid input keeps submit disabled and shows the validation hint", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("jira-import-trigger").click();

    const submit = testPage.getByTestId("jira-import-submit");
    await expect(submit).toBeDisabled();

    // Garbage that doesn't match the JIRA_KEY_RE → hint shows on submit.
    await testPage.getByTestId("jira-import-input").fill("not-a-key");
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect(testPage.getByTestId("jira-import-error")).toContainText(
      /Paste a Jira ticket URL or key/i,
    );
  });

  test("unknown key surfaces the backend error inline", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await testPage.getByTestId("jira-import-trigger").click();

    await testPage.getByTestId("jira-import-input").fill("MISSING-99");
    await testPage.getByTestId("jira-import-submit").click();
    await expect(testPage.getByTestId("jira-import-error")).toContainText(/MISSING-99/);
  });
});
