import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";

async function readClipboard(page: Page) {
  return page.evaluate(() => navigator.clipboard.readText());
}

test.describe("Preview panel copy task URL control", () => {
  test("copies the task's detail URL and shows a confirmation that clears", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);

    const task = await apiClient.createTask(seedData.workspaceId, "Preview copy task URL", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview copy task URL");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const copyButton = previewPanel.getByTestId("task-preview-copy-url");
    await expect(copyButton).toBeVisible();
    await expect(copyButton).toHaveAttribute("aria-label", "Copy task link");

    // Hover tooltip identifies the control distinctly from the Link submenu
    // (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.3/.4), which reads plain "Link".
    await copyButton.hover();
    const tooltip = testPage.getByRole("tooltip", {
      name: "Copies the task link to the clipboard",
    });
    await expect(tooltip).toBeVisible();
    await expect(tooltip).not.toHaveText("Link");

    const status = previewPanel.getByTestId("task-preview-copy-status");
    await expect(status).toHaveAttribute("role", "status");
    await expect(status).toHaveText("");

    await copyButton.click();

    const expectedUrl = `${new URL(testPage.url()).origin}/t/${task.id}`;
    await expect.poll(() => readClipboard(testPage)).toBe(expectedUrl);

    // Visual confirmation: an accessible status announcement, distinct from
    // the button's stable accessible name (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.4).
    await expect(status).toHaveText("Task link copied");
    await expect(copyButton).toHaveAttribute("aria-label", "Copy task link");

    // Clears on its own after the bounded duration.
    await expect(status).toHaveText("", { timeout: 5_000 });
  });

  test("swaps to a distinct confirmation icon and copies again on a second click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);

    const task = await apiClient.createTask(seedData.workspaceId, "Preview copy icon swap", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview copy icon swap");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });
    const copyButton = previewPanel.getByTestId("task-preview-copy-url");
    const icon = copyButton.locator("svg");

    // Before copying: the plain copy icon, not the green confirmation check.
    await expect(icon).not.toHaveClass(/text-green-500/);

    await copyButton.click();
    const expectedUrl = `${new URL(testPage.url()).origin}/t/${task.id}`;
    await expect.poll(() => readClipboard(testPage)).toBe(expectedUrl);
    // AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.4: brief visual confirmation,
    // and its icon must read as distinct from the plain copy affordance.
    await expect(icon).toHaveClass(/text-green-500/);

    // Clicking again while still in the confirmed state issues a fresh copy.
    await testPage.evaluate(() => navigator.clipboard.writeText(""));
    await copyButton.click();
    await expect.poll(() => readClipboard(testPage)).toBe(expectedUrl);
  });
});
