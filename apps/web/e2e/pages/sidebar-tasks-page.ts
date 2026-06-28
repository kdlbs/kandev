import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Page object for the task list inside the unified AppSidebar (TaskSessionSidebar).
 * Covers cmd/shift-click multi-select and the selection-aware right-click menu.
 */
export class SidebarTasksPage {
  readonly root: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("task-sidebar");
  }

  row(taskId: string): Locator {
    return this.root.locator(`[data-testid='sidebar-task-item'][data-task-id='${taskId}']`);
  }

  rows(): Locator {
    return this.root.locator("[data-testid='sidebar-task-item']");
  }

  async cmdClick(taskId: string) {
    await this.row(taskId).click({ modifiers: ["ControlOrMeta"] });
  }

  async shiftClick(taskId: string) {
    await this.row(taskId).click({ modifiers: ["Shift"] });
  }

  async plainClick(taskId: string) {
    await this.row(taskId).click();
  }

  async rightClick(taskId: string) {
    await this.row(taskId).click({ button: "right" });
  }

  async expectSelected(taskId: string, selected = true) {
    const row = this.row(taskId);
    if (selected) {
      await expect(row).toHaveAttribute("data-multiselected", "true", { timeout: 5_000 });
    } else {
      await expect(row).not.toHaveAttribute("data-multiselected", "true", { timeout: 5_000 });
    }
  }

  async selectedCount(): Promise<number> {
    return this.root
      .locator("[data-testid='sidebar-task-item'][data-multiselected='true']")
      .count();
  }

  // --- right-click bulk menu ---
  bulkArchiveMenuItem(count: number): Locator {
    return this.page.getByRole("menuitem", { name: `Archive ${count} tasks` });
  }

  singleArchiveMenuItem(): Locator {
    return this.page.getByRole("menuitem", { name: "Archive", exact: true });
  }

  bulkArchiveConfirm(): Locator {
    return this.page.getByTestId("sidebar-bulk-archive-confirm");
  }
}
