import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  readDialogRenderingMetrics,
  expectWebkitDialogMotion,
} from "../../helpers/dialog-webkit-metrics";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

async function openCreateTaskDialog(
  testPage: Page,
  beforeOpen?: () => Promise<void>,
): Promise<Locator> {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await beforeOpen?.();
  await kanban.createTaskButton.first().click();
  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Close", exact: true })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
  await expect(dialog).toHaveCSS("padding-top", "0px");
  return dialog;
}

test.describe("Create Task WebKit rendering", () => {
  test("keeps the existing Chromium motion and geometry", async ({ testPage, prCapture }) => {
    const dialog = await openCreateTaskDialog(testPage);
    await expect(testPage.locator("html")).toHaveAttribute("data-rendering-engine", "other");

    const metrics = await readDialogRenderingMetrics(dialog, {
      contentSelector: '[data-testid="create-task-dialog"]',
    });
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(metrics.animationName).toBe("enter");
    expect(metrics.translate).toContain("-50%");
    expect(metrics.zIndex).toBe("50");
    expect(metrics.overlayZIndex).toBe("50");
    expect(metrics.width).toBe(900);
    expect(metrics.centerX).toBeCloseTo(viewport!.width / 2, 0);
    expect(metrics.centerY).toBeCloseTo(viewport!.height / 2, 0);
    expect(metrics.contentOverOverlay).toBe(true);
    await prCapture.screenshot("chromium-create-task-dialog", {
      caption: "Chromium keeps the existing Create Task dialog motion and geometry.",
    });
  });

  test("uses transform-free motion and centering for WebKit", async ({ testPage, prCapture }) => {
    const dialog = await openCreateTaskDialog(testPage, async () => {
      await testPage.locator("html").evaluate((root) => {
        root.setAttribute("data-rendering-engine", "webkit");
      });
    });
    const metrics = await expectWebkitDialogMotion(dialog, {
      contentSelector: '[data-testid="create-task-dialog"]',
      contentZIndex: "50",
      overlayZIndex: "49",
    });
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(metrics.width).toBe(900);
    expect(metrics.centerX).toBeCloseTo(viewport!.width / 2, 0);
    expect(metrics.centerY).toBeCloseTo(viewport!.height / 2, 0);
    expect(metrics.contentOverOverlay).toBe(true);
    await prCapture.screenshot("webkit-create-task-dialog", {
      caption: "The WebKit-safe Create Task dialog is centered without transformed text.",
    });
  });
});
