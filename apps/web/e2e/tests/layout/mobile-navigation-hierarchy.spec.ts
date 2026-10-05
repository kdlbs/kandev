import { test, expect } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import { MobileGitHubPage } from "../../pages/mobile-github-page";

test("Open automations is a child destination that dismisses the phone menu", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Daily accessibility check",
    workflowId: seedData.workflowId,
    workflowStepId: seedData.startStepId,
  });
  await testPage.goto("/tasks");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const group = menu.getByTestId("mobile-automations-section");
  const header = group.getByRole("button", { name: "Automations", exact: true });
  await expect(group.getByRole("link", { name: "Open automations" })).toHaveCount(0);
  await header.tap();
  await expect(header).toHaveAttribute("aria-expanded", "true");
  const body = group.locator("#mobile-automations-body");
  await expect(body.getByRole("link", { name: /Daily accessibility check/ })).toBeVisible();
  const all = body.getByRole("link", { name: "Open automations" });
  await expectTouchControl(all);
  await expect(all).toHaveText("Open automations");
  await all.tap();
  await expect(menu).toBeHidden();
  await expect(testPage).toHaveURL(/\/automations$/);
});

// @covers AC-UI-NAV-HIERARCHY-003.1 AC-UI-NAV-HIERARCHY-003.2 AC-UI-NAV-HIERARCHY-003.3 AC-UI-NAV-HIERARCHY-003.4 AC-UI-NAV-HIERARCHY-003.5 AC-UI-NAV-HIERARCHY-003.6
test("tools precede long task lists and Issues is reachable from Home and a task", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubSetUser("compass-designer");
  await apiClient.mockGitHubAddIssues([
    {
      number: 125,
      title: "Improve issue search",
      state: "open",
      author_login: "compass-designer",
      assignees: ["compass-designer"],
      repo_owner: "compass",
      repo_name: "app",
    },
  ]);
  const task = await apiClient.seedTask(seedData.workspaceId, "Improve workspace navigation", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  for (let index = 0; index < 20; index++) {
    await apiClient.seedTask(seedData.workspaceId, `Compass accessibility check ${index}`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
  }
  for (const route of ["/tasks", `/t/${task.task_id}`]) {
    await testPage.goto(route);
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const picker = menu.getByTestId("mobile-workspace-trigger");
    const primary = menu.getByTestId("mobile-new-task-button");
    const tools = menu.getByRole("button", { name: "Integrations", exact: true });
    const tasks = menu.getByTestId("mobile-navigation-tasks-toggle");
    await expectTouchControl(primary);
    await expect(menu.getByRole("button", { name: "New task", exact: true })).toHaveCount(0);
    for (const width of [360, 393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await menu.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) =>
              Number.isFinite(animation.effect?.getComputedTiming().iterations),
            )
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      expect((await tools.boundingBox())!.y).toBeLessThan((await tasks.boundingBox())!.y);
      const y = (await picker.boundingBox())!.y;
      await menu.locator("nav").evaluate((el) => {
        el.scrollTop = el.scrollHeight;
      });
      expect((await picker.boundingBox())!.y).toBeCloseTo(y);
      await menu.locator("nav").evaluate((el) => {
        el.scrollTop = 0;
      });
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
    }
    await tools.tap();
    await menu.getByRole("link", { name: "GitHub", exact: true }).tap();
    await expect(menu).toBeHidden();
    const github = new MobileGitHubPage(testPage);
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
    await expect(github.issueRowByTitle("Improve issue search")).toBeVisible();
  }
});

test("creation stays open across the phone boundary with its draft", async ({ testPage }) => {
  await testPage.goto("/tasks");
  await testPage.getByTestId("app-nav-trigger").tap();
  await testPage.getByTestId("mobile-new-task-button").tap();
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  const title = dialog.getByTestId("task-title-input");
  await title.fill("Preserve this navigation draft");
  for (const width of [767, 768, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    await expect(dialog).toHaveCount(1);
    await expect(title).toHaveValue("Preserve this navigation draft");
  }
});
