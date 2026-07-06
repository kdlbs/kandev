import { expect, type Locator, type Page } from "@playwright/test";

const plural = (count: number) => (count === 1 ? "task" : "tasks");

/**
 * Page object for the task list inside the unified AppSidebar (TaskSessionSidebar).
 * Covers cmd/shift-click multi-select and the selection-aware right-click menu.
 */
export class SidebarTasksPage {
  readonly root: Locator;

  constructor(private readonly page: Page) {
    // Anchor to a single active sidebar — dock/mobile layouts can mount more
    // than one `task-sidebar` and a stale one must not satisfy these reads.
    this.root = page.getByTestId("task-sidebar").first();
  }

  row(taskId: string): Locator {
    // The per-task id lives on the sortable-task-block wrapper; the selection
    // state (data-multiselected) lives on the sidebar-task-item row inside it.
    return this.root
      .locator(`[data-testid='sortable-task-block'][data-task-id='${taskId}']`)
      .getByTestId("sidebar-task-item")
      .first();
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
    return this.page.getByRole("menuitem", { name: `Archive ${count} ${plural(count)}` });
  }

  singleArchiveMenuItem(): Locator {
    return this.page.getByRole("menuitem", { name: "Archive", exact: true });
  }

  bulkArchiveConfirm(): Locator {
    return this.page.getByTestId("sidebar-bulk-archive-confirm");
  }

  bulkPinMenuItem(count: number): Locator {
    return this.page.getByRole("menuitem", { name: `Pin ${count} ${plural(count)}` });
  }

  bulkUnpinMenuItem(count: number): Locator {
    return this.page.getByRole("menuitem", { name: `Unpin ${count} ${plural(count)}` });
  }

  bulkDeleteMenuItem(count: number): Locator {
    return this.page.getByRole("menuitem", { name: `Delete ${count} ${plural(count)}` });
  }

  bulkDeleteConfirm(): Locator {
    return this.page.getByTestId("sidebar-bulk-delete-confirm");
  }

  menuItem(name: string): Locator {
    return this.page.getByRole("menuitem", { name, exact: true });
  }
}
