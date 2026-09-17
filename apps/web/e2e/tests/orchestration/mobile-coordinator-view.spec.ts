import { test, expect } from "../../fixtures/test-base";
import { resetWorkspaceOrchestrators } from "../../helpers/orchestration";

test("mobile coordinator uses touch tabs, one pane, retained drafts and native task navigation", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "true",
    KANDEV_FEATURES_PERSONAL_ASSISTANT: "false",
    KANDEV_FEATURES_OFFICE: "false",
  });
  const ws = seedData.workspaceId;
  await resetWorkspaceOrchestrators(page.request, backend.baseUrl, ws);
  const task = await apiClient.createTask(ws, "Prepare a sample checklist", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await page.goto(`/settings/workspaces/${ws}/orchestration/new`);
  await page.getByTestId("orchestrator-profile").tap();
  await page.getByRole("option").first().tap();
  await page.getByTestId("persona-executor-profile").tap();
  await page.getByRole("option").filter({ hasNotText: "Inherit" }).first().tap();
  await page.getByRole("button", { name: "Add orchestrator", exact: true }).tap();
  await expect(page).not.toHaveURL(/\/new$/);
  const chief = new URL(page.url()).pathname.split("/").pop()!;
  await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
  const tasksTab = page.getByRole("tab", { name: "Tasks", exact: true });
  const chatTab = page.getByRole("tab", { name: "Chat", exact: true });
  await expect(tasksTab).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId(`coordinator-task-${task.id}`)).toBeVisible();
  const rowBox = await page.getByTestId(`coordinator-task-${task.id}`).boundingBox();
  expect(rowBox!.y + rowBox!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  for (const tab of [tasksTab, chatTab]) {
    const box = await tab.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(
      await tab.evaluate((el) => {
        const b = el.getBoundingClientRect();
        return el.contains(document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2));
      }),
    ).toBe(true);
  }
  await prCapture.screenshot("mobile-tasks", {
    caption: "Mobile task view with native task groups and touch controls. Synthetic data only.",
  });
  await chatTab.tap();
  const chat = page.getByTestId("orchestrator-conversation");
  await expect(chat).toBeVisible();
  await chat.locator("textarea").fill("Summarize the sample checklist.");
  await tasksTab.tap();
  await expect(chat).toBeHidden();
  await chatTab.tap();
  await expect(chat.locator("textarea")).toHaveValue("Summarize the sample checklist.");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await prCapture.screenshot("mobile-chat", {
    caption: "The same coordinator chat and generic draft on a touch device.",
  });
  await tasksTab.tap();
  await page.getByTestId(`coordinator-task-${task.id}`).getByRole("link").tap();
  await expect(page).toHaveURL(new RegExp(`/t/${task.id}`));
});
