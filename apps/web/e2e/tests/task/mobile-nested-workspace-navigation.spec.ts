import { test, expect } from "../../fixtures/test-base";
import { anchorTask, surface } from "./sidebar-shared-task-state-fixtures";

test("nested workspace menu releases the Tasks drawer lock on both navigation directions", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const other = await apiClient.createWorkspace("Nested workspace B");
  const { workflows } = await apiClient.listWorkflows(other.id);
  const workflow = workflows.find((item) => !item.hidden)!;
  const { steps } = await apiClient.listWorkflowSteps(workflow.id);
  const step = steps.find((item) => item.is_start_step)!;
  const first = await anchorTask(apiClient, seedData);
  const second = await anchorTask(apiClient, {
    ...seedData,
    workspaceId: other.id,
    workflowId: workflow.id,
    startStepId: step.id,
  });
  await testPage.goto(`/t/${first.id}`);
  const originalPointerEvents = await testPage.evaluate(() => document.body.style.pointerEvents);
  const workspaceAName = await apiClient
    .listWorkspaces()
    .then(({ workspaces }) => workspaces.find((item) => item.id === seedData.workspaceId)!.name);
  for (const destination of [
    { task: second, name: other.name },
    { task: first, name: workspaceAName },
  ]) {
    const { rows } = await surface(testPage, true);
    await expect
      .poll(() => testPage.evaluate(() => document.body.style.pointerEvents))
      .toBe("none");
    await rows.getByTitle("Switch workspace").tap();
    const menu = testPage.getByRole("menu");
    await expect(menu).toHaveCount(1);
    await expect(menu.getByRole("menuitem", { name: destination.name, exact: true })).toBeVisible();
    await testPage.screenshot({ path: testInfo.outputPath(`menu-${destination.task.id}.png`) });
    await menu.getByRole("menuitem", { name: destination.name, exact: true }).tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${destination.task.id}$`));
    await expect(rows).toBeHidden();
    await expect
      .poll(() => testPage.evaluate(() => document.body.style.pointerEvents))
      .toBe(originalPointerEvents);
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toBeEnabled();
    await testPage.screenshot({
      path: testInfo.outputPath(`destination-${destination.task.id}.png`),
    });
  }
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  await expect(testPage.getByRole("dialog", { name: "Tasks", exact: true })).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("dialog", { name: "Tasks", exact: true })).toBeHidden();
  await expect(testPage.getByTestId("mobile-task-picker-trigger")).toBeFocused();
  await expect
    .poll(() => testPage.evaluate(() => document.body.style.pointerEvents))
    .toBe(originalPointerEvents);
});

test("nested workspace menu dismisses to its opener before the Tasks drawer", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await apiClient.createWorkspace("Keyboard workspace B");
  const task = await anchorTask(apiClient, seedData);
  await testPage.goto(`/t/${task.id}`);
  const { rows } = await surface(testPage, true);
  const opener = rows.getByTitle("Switch workspace");
  await opener.tap();
  const menu = testPage.getByRole("menu");
  await expect(menu).toBeVisible();
  await testPage.keyboard.press("ArrowDown");
  await expect(menu.getByRole("menuitem").nth(0)).toBeFocused();
  await testPage.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(rows).toBeVisible();
  await expect(opener).toBeFocused();
  await expect.poll(() => testPage.evaluate(() => document.body.style.pointerEvents)).toBe("none");
  await testPage.screenshot({ path: testInfo.outputPath("menu-dismissed-drawer-retained.png") });
  await testPage.keyboard.press("Enter");
  await expect(menu).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await testPage.keyboard.press("Escape");
  await expect(rows).toBeHidden();
  await expect(testPage.getByTestId("mobile-task-picker-trigger")).toBeFocused();
  await expect
    .poll(() => testPage.evaluate(() => document.body.style.pointerEvents))
    .not.toBe("none");
});
