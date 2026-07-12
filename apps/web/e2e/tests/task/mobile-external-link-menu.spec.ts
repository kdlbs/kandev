import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar — external link menu", () => {
  test("shows enabled integration link actions in the task switcher sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    const sentry = await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Sentry",
      secret: "sntrys_xxx",
    });
    await apiClient.mockSentrySetAuthHealth({ instanceId: sentry.id, ok: true });
    await Promise.all([
      apiClient.waitForIntegrationAuthHealthy("jira"),
      apiClient.waitForIntegrationAuthHealthy("linear"),
      apiClient.waitForIntegrationAuthHealthy("sentry", { workspaceId: seedData.workspaceId }),
    ]);

    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile external link task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");
    const taskRow = sheet.locator('[role="button"]').filter({
      hasText: "Mobile external link task",
    });
    await expect(taskRow).toBeVisible({ timeout: 10_000 });
    await taskRow.getByRole("button", { name: "Task actions" }).click();

    const linkTrigger = testPage.getByRole("menuitem", { name: /^Link$/ });
    await expect(linkTrigger).toBeVisible();
    await linkTrigger.focus();
    await testPage.keyboard.press("ArrowRight");

    await expect(testPage.getByRole("menuitem", { name: "Jira Ticket" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Linear Issue" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Sentry Issue" })).toBeVisible();
  });
});
