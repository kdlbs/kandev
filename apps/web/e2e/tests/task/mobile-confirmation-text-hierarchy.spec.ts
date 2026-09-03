import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

const LONG_TASK_TITLE = `Mobile cleanup confirmation ${"X".repeat(32)}`;

async function enablePseudoLocale(page: Page): Promise<void> {
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });
}

async function openDeleteDialog(page: Page, title: string) {
  const session = new SessionPage(page);
  await session.mobileSessionMenu.tap();

  const drawer = page
    .getByTestId("mobile-task-switcher-list")
    .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
  await expect(drawer).toBeVisible();

  const taskRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(taskRow).toBeVisible();
  await taskRow.locator("button.mobile-task-actions-button").tap();

  const menu = page.locator('[data-slot="context-menu-content"]:visible').last();
  await expect(menu).toBeVisible();
  const deleteItem = menu.locator('[role="menuitem"]:has(svg.tabler-icon-trash)');
  await expect(deleteItem).toHaveCount(1);
  await deleteItem.tap();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  return { dialog, drawer, taskRow };
}

async function dialogMetrics(dialog: Locator) {
  return dialog.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const scrollOwners = Array.from(element.querySelectorAll<HTMLElement>("*"))
      .filter((candidate) => {
        const style = getComputedStyle(candidate);
        return (
          (style.overflowY === "auto" || style.overflowY === "scroll") &&
          candidate.scrollHeight > candidate.clientHeight + 1
        );
      })
      .map((candidate) => candidate.dataset.slot ?? candidate.tagName.toLowerCase());
    const style = getComputedStyle(element);
    return {
      x: rect.x,
      y: rect.y,
      right: rect.right,
      bottom: rect.bottom,
      width: rect.width,
      height: rect.height,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      rootOverflowY: style.overflowY,
      scrollOwners,
    };
  });
}

async function assertPhoneDeleteDialog(dialog: Locator, width: number): Promise<void> {
  await waitForFiniteAnimations(dialog);

  const title = dialog.locator('[data-slot="alert-dialog-title"]');
  const description = dialog.locator('[data-slot="alert-dialog-description"]');
  await expect(title).toBeVisible();
  await expect(description).toBeVisible();

  await expect(title).toHaveCSS("text-wrap", "balance");
  await expect(description).toHaveCSS("text-wrap", "pretty");
  await expect(description).toHaveCSS("text-align", "left");
  await expect(description).toHaveCSS("overflow-wrap", /anywhere|break-word/);

  const labelledBy = await dialog.getAttribute("aria-labelledby");
  const describedBy = await dialog.getAttribute("aria-describedby");
  expect(labelledBy).toBe(await title.getAttribute("id"));
  expect(describedBy).toBe(await description.getAttribute("id"));
  expect(await description.locator("p").count()).toBeGreaterThan(0);
  expect(await description.locator("ul, ol").count()).toBeGreaterThan(0);

  await assertLocatorWithinViewportX(dialog, `phone delete dialog at ${width}px`);
  await assertNoDescendantOverflowsRight(dialog, `phone delete dialog at ${width}px`);
  await assertNoDocumentHorizontalOverflow(pageFor(dialog), `phone delete dialog at ${width}px`);

  const metrics = await dialogMetrics(dialog);
  expect(metrics.x).toBeGreaterThanOrEqual(15);
  expect(metrics.right).toBeLessThanOrEqual(metrics.viewportWidth - 15);
  expect(metrics.y).toBeGreaterThanOrEqual(0);
  expect(metrics.bottom).toBeLessThanOrEqual(metrics.viewportHeight);
  expect(metrics.rootOverflowY).not.toMatch(/auto|scroll/);
  if (width === 320) {
    expect(metrics.scrollOwners.length).toBeGreaterThan(0);
  }

  const footer = dialog.locator('[data-slot="alert-dialog-footer"]');
  const cancel = dialog.locator('[data-slot="alert-dialog-cancel"]');
  const deleteAction = dialog.locator('[data-slot="alert-dialog-action"]');
  await expect(cancel).toBeVisible();
  await expect(deleteAction).toBeVisible();

  const [footerBox, cancelBox, deleteBox] = await Promise.all([
    footer.boundingBox(),
    cancel.boundingBox(),
    deleteAction.boundingBox(),
  ]);
  if (!footerBox || !cancelBox || !deleteBox) {
    throw new Error("phone delete dialog footer has no rendered action boxes");
  }
  expect(Math.round(cancelBox.height)).toBeGreaterThanOrEqual(44);
  expect(Math.round(deleteBox.height)).toBeGreaterThanOrEqual(44);
  expect(cancelBox.width).toBeGreaterThanOrEqual(footerBox.width - 2);
  expect(deleteBox.width).toBeGreaterThanOrEqual(footerBox.width - 2);
  await expect(cancel).toBeInViewport();
  await expect(deleteAction).toBeInViewport();
  await expect(deleteAction).toHaveAttribute("data-variant", "destructive");
  await expect(deleteAction).not.toHaveClass(/bg-primary/);
}

function pageFor(locator: Locator): Page {
  return locator.page();
}

test.describe("Mobile confirmation text hierarchy", () => {
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.3, AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.5, AC-UI-TASK-CLEANUP-CONFIRMATION-001.6
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
  test("keeps a pseudo-localized delete confirmation contained at 320px and 393px", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 320, height: 400 });
    const task = await apiClient.seedTask(seedData.workspaceId, LONG_TASK_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTask(seedData.workspaceId, "Mobile cleanup confirmation child", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: task.task_id,
    });

    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await enablePseudoLocale(testPage);

    for (const width of [320, 393]) {
      await testPage.setViewportSize({ width, height: 400 });
      const { dialog, drawer } = await openDeleteDialog(testPage, LONG_TASK_TITLE);
      const drawerTitle = drawer.locator('[data-slot="drawer-title"]');
      await expect(drawerTitle).toBeVisible();
      await expect(drawerTitle).toHaveCSS("text-wrap", "balance");
      await assertPhoneDeleteDialog(dialog, width);

      await dialog.locator('[data-slot="alert-dialog-cancel"]').tap();
      await expect(dialog).toBeHidden();
      expect((await apiClient.getTask(task.task_id)).id).toBe(task.task_id);
    }

    const { dialog } = await openDeleteDialog(testPage, LONG_TASK_TITLE);
    await dialog.locator('[data-slot="alert-dialog-action"]').tap();
    await expect
      .poll(
        async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${task.task_id}`)).status,
        { timeout: 15_000, message: "the mobile Delete action should remove the task" },
      )
      .toBe(404);
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  // @covers AC-UI-TASK-CLEANUP-CONFIRMATION-001.4, AC-UI-TASK-CLEANUP-CONFIRMATION-001.5
  test("keeps the phone Drawer and inline archive confirmation touch-safe", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 640 });
    const title = "Mobile inline archive confirmation target";
    const task = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    const session = new SessionPage(testPage);
    await session.mobileSessionMenu.tap();

    const drawer = testPage
      .getByTestId("mobile-task-switcher-list")
      .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
    await expect(drawer).toBeVisible();
    const drawerTitle = drawer.locator('[data-slot="drawer-title"]');
    await expect(drawerTitle).toHaveCSS("text-wrap", "balance");

    const taskRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: title });
    await expect(taskRow).toBeVisible();
    await taskRow.locator("button.mobile-task-actions-button").tap();
    const menu = testPage.locator('[data-slot="context-menu-content"]:visible').last();
    await menu.getByRole("menuitem", { name: "Archive", exact: true }).tap();

    const confirmation = taskRow.getByTestId("task-archive-inline-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(confirmation).toContainText(title);
    await assertNoDocumentHorizontalOverflow(testPage, "phone inline archive confirmation");

    for (const button of [
      confirmation.getByRole("button", { name: "Cancel", exact: true }),
      confirmation.getByTestId("archive-task-confirm"),
    ]) {
      const box = await button.boundingBox();
      if (!box) throw new Error("phone inline archive action has no rendered hitbox");
      expect(Math.round(box.width)).toBeGreaterThanOrEqual(44);
      expect(Math.round(box.height)).toBeGreaterThanOrEqual(44);
      await expect(button).toBeInViewport();
    }

    await confirmation.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(confirmation).toBeHidden();
    expect((await apiClient.getTask(task.task_id)).id).toBe(task.task_id);
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  test("uses balanced Alert titles and pretty inline Alert prose on a phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 320, height: 480 });
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile runtime alert target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await stubAgentRuntimeRestart(testPage);
    await testPage.goto(`/t/${task.task_id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    await setAgentRuntimeAvailability(testPage, {
      status: "unavailable",
      reason: "agentctl_exited",
      occurred_at: "2026-08-08T14:22:52Z",
    });
    const alert = testPage.getByTestId("agent-runtime-alert");
    await expect(alert).toBeVisible();
    const title = alert.locator('[data-slot="alert-title"]');
    const description = alert.locator('[data-slot="alert-description"]');
    await expect(title).toHaveCSS("text-wrap", "balance");
    await expect(description).toHaveCSS("text-wrap", "pretty");
    await assertNoDescendantOverflowsRight(alert, "phone runtime Alert");
    await assertNoDocumentHorizontalOverflow(testPage, "phone runtime Alert");

    const action = alert.getByRole("button").first();
    await expect(action).toBeVisible({ timeout: 15_000 });
    const actionBox = await action.boundingBox();
    if (!actionBox) throw new Error("phone runtime Alert action has no rendered hitbox");
    expect(Math.round(actionBox.height)).toBeGreaterThanOrEqual(44);
    await expect(action).toBeInViewport();

    await setAgentRuntimeAvailability(testPage, { status: "available" });
    await expect(alert).toHaveCount(0);
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  test("keeps the mobile board Drawer title and description inside the viewport", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 640 });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.boardNavigator.tap();

    const drawer = testPage.getByTestId("mobile-board-navigator-drawer");
    await expect(drawer).toBeVisible();
    const title = drawer.locator('[data-slot="drawer-title"]');
    const description = drawer.locator('[data-slot="drawer-description"]');
    await expect(title).toHaveCSS("text-wrap", "balance");
    await expect(description).toHaveCSS("text-wrap", "pretty");
    await assertLocatorWithinViewportX(drawer, "mobile board navigator Drawer");
    await assertNoDescendantOverflowsRight(drawer, "mobile board navigator Drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile board navigator Drawer");
  });
});
