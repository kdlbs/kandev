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
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet.getByTestId("sidebar-task-item").filter({
      hasText: "Mobile external link task",
    });
    await expect(taskRow).toBeVisible({ timeout: 10_000 });
    const actions = taskRow.getByRole("button", { name: "Task actions" });
    await expect(actions).toBeVisible();
    await actions.click();

    const linkTrigger = testPage.getByRole("menuitem", { name: /^Link$/ });
    await expect(linkTrigger).toBeVisible();
    await linkTrigger.click();

    const jiraItem = testPage.getByRole("menuitem", { name: "Jira Ticket" });
    await expect(jiraItem).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Linear Issue" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Sentry Issue" })).toBeVisible();

    const nestedMenu = jiraItem.locator("xpath=ancestor::*[@role='menu'][1]");
    await nestedMenu.evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );
    const [menuBox, itemBox] = await Promise.all([
      nestedMenu.boundingBox(),
      jiraItem.boundingBox(),
    ]);
    const viewport = testPage.viewportSize();
    if (!menuBox || !itemBox || !viewport) throw new Error("mobile nested menu has no layout box");
    expect(menuBox.x).toBeGreaterThanOrEqual(8);
    expect(menuBox.x).toBeLessThanOrEqual(18);
    expect(menuBox.width).toBeGreaterThanOrEqual(viewport.width - 36);
    expect(viewport.width - (menuBox.x + menuBox.width)).toBeGreaterThanOrEqual(8);
    expect(viewport.width - (menuBox.x + menuBox.width)).toBeLessThanOrEqual(18);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeGreaterThanOrEqual(7);
    expect(viewport.height - (menuBox.y + menuBox.height)).toBeLessThanOrEqual(18);
    expect(itemBox.height).toBeGreaterThanOrEqual(44);
  });
});
