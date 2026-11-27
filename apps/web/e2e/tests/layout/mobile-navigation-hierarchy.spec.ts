import { test, expect } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import { MobileGitHubPage } from "../../pages/mobile-github-page";
import { closeQuickTerminalTab } from "../terminal/terminal-test-helpers";
import { watchWs } from "../../helpers/causal-waits";
import type { AutomationRun } from "@/lib/types/automation";
import { summaryHasRun } from "../../helpers/automation-summary-waits";
import ptAutomations from "../../../src/locales/pt-pt/automations.json";

function rightEdge(box: { x: number; width: number }): number {
  return box.x + box.width;
}

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

test("phone drawer actions keep touch targets, focus handoff, and direct Stats access", async ({
  testPage,
}) => {
  test.setTimeout(120_000);
  await testPage.goto("/tasks");
  const trigger = testPage.getByTestId("app-nav-trigger");

  for (const width of [360, 393, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    await trigger.tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    await expect(menu).toBeVisible();
    const actionIds = [
      "mobile-new-task-button",
      "mobile-quick-chat-button",
      "mobile-quick-terminal-button",
    ];
    for (const id of actionIds) {
      const action = menu.getByTestId(id);
      await expectTouchControl(action);
    }
    const { menuBox, actionBoxes } = await menu.evaluate((element, ids) => {
      const box = (node: Element) => {
        const { x, y, right, bottom } = node.getBoundingClientRect();
        return { x, y, right, bottom };
      };
      return {
        menuBox: box(element),
        actionBoxes: ids.map((id) => {
          const action = element.querySelector(`[data-testid="${id}"]`);
          if (!action) throw new Error(`Missing menu action: ${id}`);
          return box(action);
        }),
      };
    }, actionIds);
    for (const box of actionBoxes) {
      expect(box.x).toBeGreaterThanOrEqual(menuBox.x);
      expect(box.right).toBeLessThanOrEqual(menuBox.right);
      expect(box.y).toBeGreaterThanOrEqual(menuBox.y);
      expect(box.bottom).toBeLessThanOrEqual(menuBox.bottom);
    }
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await menu.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
  }

  await testPage.setViewportSize({ width: 393, height: 851 });
  await trigger.tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await menu.getByTestId("mobile-new-task-button").tap();
  const createDialog = testPage.getByTestId("create-task-dialog");
  await expect(menu).toBeHidden();
  await expect(createDialog).toBeVisible();
  await createDialog.getByRole("button", { name: "Cancel", exact: true }).tap();
  await expect(createDialog).toBeHidden();

  await trigger.tap();
  await menu.getByTestId("mobile-quick-chat-button").tap();
  const quickChat = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
  await expect(menu).toBeHidden();
  await expect(quickChat.getByTestId("quick-chat-setup")).toBeVisible();
  await quickChat.getByTestId("quick-chat-close").tap();
  await expect(quickChat).toBeHidden();
  await expect(trigger).toBeFocused();

  await trigger.tap();
  await menu.getByTestId("mobile-quick-terminal-button").tap();
  // Quick Terminal is a tab in the shared Quick Chat dialog host.
  const quickTerminal = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
  await expect(menu).toBeHidden();
  const terminalTabs = quickTerminal.getByTestId("quick-terminal-tab");
  await expect(terminalTabs).toHaveCount(1);
  await expect(quickTerminal.getByTestId("quick-terminal-terminal")).toBeVisible();
  await closeQuickTerminalTab(testPage, terminalTabs.first());
  await expect(terminalTabs).toHaveCount(0);
  await expect(quickTerminal).toBeHidden();
  await expect(trigger).toBeFocused();

  await trigger.tap();
  const utilities = menu.getByText("Utilities", { exact: true });
  const stats = menu.getByRole("link", { name: "Stats", exact: true });
  await expect(utilities).toBeVisible();
  await stats.scrollIntoViewIfNeeded();
  await expectTouchControl(stats);
  const menuBox = (await menu.boundingBox())!;
  const statsBox = (await stats.boundingBox())!;
  expect(statsBox.x).toBeGreaterThanOrEqual(menuBox.x);
  expect(rightEdge(statsBox)).toBeLessThanOrEqual(rightEdge(menuBox));
  await stats.tap();
  await expect(menu).toBeHidden();
  await expect(testPage).toHaveURL(/\/stats$/);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
    true,
  );
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.20 AC-OFFICE-AUTOMATION-RUNS-001.21 AC-OFFICE-AUTOMATION-RUNS-001.22
test("phone automation rows expose latest failure and keep row-tap navigation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const automations = [];
  for (const name of ["PR tagging", "Paused monitor", "Long localized retry outcome"]) {
    automations.push(
      await apiClient.seedAutomation({
        workspaceId: seedData.workspaceId,
        name,
        workflowId: seedData.workflowId,
        workflowStepId: seedData.startStepId,
      }),
    );
  }
  const [automation, paused, retry] = automations;
  await apiClient.wsRequest("automation.update", { id: paused.id, enabled: false });
  const failed = await apiClient.seedAutomationRun(automation.id, "failed");
  await apiClient.seedAutomationRun(paused.id, "failed");
  await apiClient.seedAutomationRun(retry.id, "retry_scheduling_failed");
  const ws = watchWs(testPage);
  await testPage.goto("/tasks");
  const loaded = ws.waitForResponse("automation.summaries", {
    where: (payload) => summaryHasRun(payload, failed.id),
  });
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const group = menu.getByTestId("mobile-automations-section");
  await group.getByRole("button", { name: "Automations", exact: true }).tap();
  await loaded;
  const row = group.getByRole("link", { name: "PR tagging", exact: true });
  await expect(row.locator(".bg-red-500")).toBeVisible();
  await expect(row).toContainText("Failed");
  await expect(row).toHaveAccessibleDescription("Failed.");
  const pausedRow = group.getByRole("link", { name: "Paused monitor", exact: true });
  await expect(pausedRow.locator(".bg-muted-foreground\\/50")).toBeVisible();
  await expect(pausedRow).toContainText("Paused");
  await expect(pausedRow).toContainText("Failed");
  await expectTouchControl(row);
  await group.getByRole("link", { name: "Open automations", exact: true }).scrollIntoViewIfNeeded();
  await prCapture.screenshot("automation-latest-failure-phone", {
    caption: "UI-03: active saved-layout phone rows expose failure, including a paused-gray row",
  });
  const original = await menu.elementHandle();
  const timeOrigin = await testPage.evaluate(() => performance.timeOrigin);
  const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
    automation_id: automation.id,
  });
  await expect.poll(() => Date.now()).toBeGreaterThan(Date.parse(runs[0].created_at) + 1000);
  let successId = "";
  const refreshed = ws.waitForResponse("automation.summaries", {
    where: (payload) => summaryHasRun(payload, successId, "succeeded"),
  });
  successId = (await apiClient.seedAutomationRun(automation.id, "succeeded")).id;
  await refreshed;
  await expect(menu).toBeVisible();
  await expect(row.locator(".bg-emerald-500")).toBeVisible();
  await expect(row).toContainText("Succeeded");
  await expect(row).not.toContainText("Failed");
  expect(await menu.evaluate((element, previous) => element === previous, original)).toBe(true);
  expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(timeOrigin);
  await expect(row).toHaveAttribute("href", `/automations/${automation.id}`);
  const newest = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
    automation_id: automation.id,
  });
  expect(newest[0].id).toBe(successId);
  expect(Date.parse(newest[0].created_at)).toBeGreaterThan(Date.parse(runs[0].created_at));
  await original?.dispose();
  await row.tap();
  await expect(menu).toBeHidden();
  await expect(testPage).toHaveURL(new RegExp(`/automations/${automation.id}$`));
  await testPage
    .context()
    .addCookies([
      { name: "kandev_locale", value: "pt-pt", url: `${new URL(testPage.url()).origin}/` },
    ]);
  await testPage.goto("/tasks");
  await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");
  await testPage.getByTestId("app-nav-trigger").tap();
  await group.getByRole("button", { name: ptAutomations.automations, exact: true }).tap();
  const longRow = group.getByRole("link", { name: "Long localized retry outcome", exact: true });
  const longStatus = longRow.getByText(ptAutomations.runStatusRetrySchedulingFailed, {
    exact: true,
  });
  await longRow.scrollIntoViewIfNeeded();
  await expect(longStatus).toBeVisible();
  await expectTouchControl(longRow);
  const geometry = await longRow.evaluate((element) => {
    const status = element.querySelector('[data-testid="mobile-automation-status"]')!;
    const name = element.querySelector('[data-testid="mobile-automation-name"]')!;
    const box = element.getBoundingClientRect();
    const statusBox = status.getBoundingClientRect();
    const nameBox = name.getBoundingClientRect();
    return {
      height: box.height,
      right: box.right,
      statusRight: statusBox.right,
      statusLeft: statusBox.left,
      nameRight: nameBox.right,
      statusWidth: status.clientWidth,
      statusScrollWidth: status.scrollWidth,
      statusHeight: statusBox.height,
    };
  });
  expect(geometry.height).toBeGreaterThanOrEqual(44);
  expect(geometry.statusScrollWidth).toBeLessThanOrEqual(geometry.statusWidth);
  expect(geometry.statusRight).toBeLessThanOrEqual(geometry.right);
  expect(geometry.nameRight).toBeLessThanOrEqual(geometry.statusLeft);
  expect(geometry.statusHeight).toBeGreaterThan(20);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
    true,
  );
  await group
    .getByRole("link", { name: ptAutomations.openAutomations, exact: true })
    .scrollIntoViewIfNeeded();
  await prCapture.screenshot("automation-latest-failure-phone-pt", {
    caption: "UI-03: long Portuguese retry failure wraps within the phone row",
  });
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.21 AC-OFFICE-AUTOMATION-RUNS-001.22
test("phone agenda preserves automation names beside outcome metadata", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const cases = [];
  for (const [status, label] of [
    ["scheduled_retry", "Retry scheduled"],
    ["cancelled", "Cancelled"],
  ]) {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: `Nightly dependency audit (${status})`,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "scheduled",
      config: { cron_expression: "0 9 * * *", timezone: "UTC" },
    });
    const run = await apiClient.seedAutomationRun(automation.id, status);
    cases.push({ automation, run, label });
  }
  const ws = watchWs(testPage);
  const loaded = ws.waitForResponse("automation.summaries", {
    where: (payload) => cases.every(({ run }) => summaryHasRun(payload, run.id)),
  });
  await testPage.goto("/automations");
  await loaded;
  for (const width of [360, 393, 767, 768]) {
    await testPage.setViewportSize({ width, height: 851 });
    for (const { automation, label } of cases) {
      const row = testPage.getByTestId(`agenda-row-${automation.id}`);
      await expect(row).toHaveCount(1);
      const name = row.getByText(automation.name, { exact: true });
      const outcome = row.getByText(label, { exact: true });
      await expect(name).toBeVisible();
      await expect(outcome).toBeVisible();
      const nameBox = (await name.boundingBox())!;
      const outcomeBox = (await outcome.boundingBox())!;
      expect(
        nameBox.width,
        `${label} must leave a readable name at ${width}px`,
      ).toBeGreaterThanOrEqual(64);
      expect(
        outcomeBox.y >= nameBox.y + nameBox.height - 1 ||
          outcomeBox.x >= nameBox.x + nameBox.width - 1,
      ).toBe(true);
    }
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
  await testPage.setViewportSize({ width: 393, height: 851 });
  await prCapture.screenshot("automation-agenda-readable-names-phone", {
    caption: "UI-02: phone agenda preserves automation names and wraps non-success outcomes",
  });
});
