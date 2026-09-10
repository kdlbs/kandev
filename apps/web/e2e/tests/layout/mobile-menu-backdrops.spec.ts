import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  expectMenuBackdropCount,
  expectMobileMenuBackdrop,
  expectReachableMenuItem,
} from "../../helpers/menu-backdrop";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile menu backdrops", () => {
  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.11
  test("the backdrop fades with the closing menu sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Closing backdrop task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.taskCard(task.id).getByRole("button", { name: "More options" }).tap();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"]');
    await expectMobileMenuBackdrop(menu);

    const closeFrame = await menu.evaluateHandle((element) => {
      const frame = { content: "", opacity: "", transition: "", duration: "" };
      const observer = new MutationObserver(() => {
        if (element.getAttribute("data-state") !== "closed") return;
        const backdrop = getComputedStyle(element.parentElement!, "::before");
        frame.content = backdrop.content;
        frame.opacity = backdrop.opacity;
        frame.transition = backdrop.transitionProperty;
        frame.duration = backdrop.transitionDuration;
        observer.disconnect();
      });
      observer.observe(element, { attributes: true, attributeFilter: ["data-state"] });
      return frame;
    });
    await testPage.keyboard.press("Escape");
    const closing = await closeFrame.jsonValue();
    expect(["none", "normal"]).not.toContain(closing.content);
    expect(Number.parseFloat(closing.opacity)).toBeGreaterThan(0);
    expect(closing.transition).toBe("opacity");
    expect(closing.duration).toBe("0.1s");
    await closeFrame.dispose();
    await expect(menu).toHaveCount(0);
    await expectMenuBackdropCount(testPage, 0);
  });

  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.3, AC-UI-MOBILE-TASK-NAVIGATION-001.9, AC-UI-MOBILE-TASK-NAVIGATION-001.11
  test("Kanban task options blur the background and dismiss cleanly", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile backdrop task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const trigger = mobile.taskCard(task.id).getByRole("button", { name: "More options" });
    const menu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');

    for (const colorScheme of ["light", "dark"] as const) {
      await testPage.emulateMedia({ colorScheme });
      await expect(testPage.locator("html")).toHaveClass(new RegExp(`\\b${colorScheme}\\b`));
      await mobile.mobileMenuButton.tap();
      const drawer = testPage.getByRole("dialog", { name: "Menu", exact: true });
      await expect(drawer).toBeVisible();
      const drawerBackdrop = await testPage
        .locator('[data-slot="drawer-overlay"]:visible')
        .evaluate((element) => {
          const style = getComputedStyle(element);
          return { background: style.backgroundColor, blur: style.backdropFilter };
        });
      await testPage.keyboard.press("Escape");
      await expect(drawer).toBeHidden();

      await trigger.tap();
      const backdrop = await expectMobileMenuBackdrop(menu);
      expect(backdrop.background).toBe(drawerBackdrop.background);
      expect(backdrop.blur).toBe(drawerBackdrop.blur);
      await expectMenuBackdropCount(testPage, 1);
      await expectReachableMenuItem(menu.getByRole("menuitem", { name: "Edit", exact: true }));
      await assertNoDocumentHorizontalOverflow(testPage);
      await prCapture.screenshot(`kanban-menu-${colorScheme}`, {
        caption: `Mobile task options with a blurred background in ${colorScheme} mode`,
      });

      if (colorScheme === "light") await testPage.touchscreen.tap(20, 20);
      else await testPage.keyboard.press("Escape");
      await expect(menu).toBeHidden();
      await expectMenuBackdropCount(testPage, 0);
      await expect(trigger).toBeFocused();
      await expect(testPage).toHaveURL(/\/$/);
    }

    await trigger.tap();
    await expectMenuBackdropCount(testPage, 1);
    await apiClient.deleteTask(task.id);
    await expect(mobile.taskCard(task.id)).toBeHidden();
    await expectMenuBackdropCount(testPage, 0);
    await mobile.mobileMenuButton.tap();
    await expect(mobile.menuCard).toBeVisible();
  });

  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.3, AC-UI-MOBILE-TASK-NAVIGATION-001.10, AC-UI-MOBILE-TASK-NAVIGATION-001.11
  test("task submenus share one backdrop and preserve the task drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Nested backdrop task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
    const row = drawer.getByTestId("sidebar-task-item").filter({ hasText: task.title });
    const actions = row.getByRole("button", { name: "Task actions" });
    await actions.tap();
    const rootMenu = testPage.locator('[data-slot="context-menu-content"][data-state="open"]');
    await expectMobileMenuBackdrop(rootMenu);
    await expectMenuBackdropCount(testPage, 1);

    const priority = rootMenu.getByTestId("task-context-priority");
    await priority.tap();
    const low = testPage.getByTestId("task-context-priority-low");
    await expectReachableMenuItem(low);
    await expectMenuBackdropCount(testPage, 1);
    await prCapture.screenshot("nested-task-menu", {
      caption: "A task priority submenu retains one menu backdrop above the task drawer",
    });
    await low.press("ArrowLeft");
    await expect(low).toBeHidden();
    await expect(rootMenu).toBeVisible();
    await expectMenuBackdropCount(testPage, 1);
    await priority.tap();
    await low.tap();
    await expect.poll(async () => (await apiClient.getTask(task.id)).priority).toBe("low");
    await expectMenuBackdropCount(testPage, 0);
    await expect(drawer).toBeVisible();
    await expect(testPage.locator('[data-slot="drawer-overlay"]:visible')).toHaveCount(1);
    await assertNoDocumentHorizontalOverflow(testPage);
    await actions.tap();
    await expectMobileMenuBackdrop(rootMenu);
    await priority.tap();
    await expect(low).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expectMenuBackdropCount(testPage, 0);
    await expect(drawer).toBeVisible();
  });

  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.9, AC-UI-MOBILE-TASK-NAVIGATION-001.11, AC-UI-MOBILE-TASK-NAVIGATION-001.12
  test("workspace menu preserves non-modal drawer interaction", async ({ testPage, apiClient }) => {
    const otherWorkspace = await apiClient.createWorkspace("Backdrop workspace");
    await apiClient.createWorkflow(otherWorkspace.id, "Backdrop workflow", "simple");
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.tap();
    const drawer = testPage.getByRole("dialog", { name: "Menu", exact: true });
    const trigger = testPage.getByTestId("mobile-workspace-trigger");
    await trigger.tap();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expectMobileMenuBackdrop(menu);
    await expectMenuBackdropCount(testPage, 1);
    await drawer.getByRole("heading", { name: "Menu", exact: true }).tap();
    await expect(menu).toBeHidden();
    await expectMenuBackdropCount(testPage, 0);
    await expect(drawer).toBeVisible();
    await expect(testPage.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0);
    await trigger.tap();
    await expectMobileMenuBackdrop(menu);
    const choice = testPage.getByTestId(`mobile-workspace-item-${otherWorkspace.id}`);
    await expectReachableMenuItem(choice);
    await choice.tap();
    await expect(drawer).toBeHidden();
    await expectMenuBackdropCount(testPage, 0);
    await mobile.mobileMenuButton.tap();
    await expect(trigger).toContainText(otherWorkspace.name);
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
