import { type Locator, type Page, expect } from "@playwright/test";

/**
 * Page object for the sidebar filter / view system.
 *
 * After the unified AppSidebar overhaul, the filter UI no longer renders the
 * legacy `sidebar-filter-bar`. It now lives in the AppSidebar Tasks section
 * header as the `TasksViewPicker`:
 *   - `tasks-view-picker`  — a dropdown trigger button labelled with the active
 *     view name. Opening it lists the saved views as `sidebar-view-chip` items
 *     (radix DropdownMenu, portaled). Selecting a chip switches the active view.
 *   - `sidebar-filter-gear` — opens the `sidebar-filter-popover` (filters, sort,
 *     group, save/rename/delete). Unchanged from before the overhaul.
 *
 * Notes on what changed vs. the old bar:
 *   - The old always-visible `sidebar-view-chip-row` is gone; chips are only in
 *     the open view-picker dropdown. Chip-reading helpers open it first.
 *   - The active-view label is shown on the picker trigger button.
 *   - Drag-to-reorder views was removed (the dropdown items are not sortable).
 */
export class SidebarFilterPopoverPage {
  /** The always-visible filter affordance (view picker) in the section header. */
  readonly bar: Locator;
  readonly gear: Locator;
  /** The view-picker dropdown trigger (shows the active view name). */
  readonly viewPicker: Locator;

  constructor(private readonly page: Page) {
    this.bar = page.getByTestId("tasks-view-picker");
    this.viewPicker = page.getByTestId("tasks-view-picker");
    this.gear = this.viewPicker.locator("xpath=..").getByTestId("sidebar-filter-gear");
  }

  get popover(): Locator {
    return this.page.getByTestId("sidebar-filter-popover");
  }

  /** Direct-create action inside the currently open desktop view menu. */
  get newViewAction(): Locator {
    return this.chipMenu.getByTestId("sidebar-new-view");
  }

  get dirtyIndicator(): Locator {
    return this.popover.getByTestId("sidebar-filter-dirty-indicator");
  }

  /** Open radix dropdown menu hosting the view chips. Scoped to the menu that
   *  actually contains the view chips so it can't match another sidebar menu. */
  get chipMenu(): Locator {
    return this.page.getByRole("menu").filter({ has: this.page.getByTestId("sidebar-view-chip") });
  }

  async open(): Promise<void> {
    if (!(await this.popover.isVisible())) {
      await this.gear.click();
      await expect(this.popover).toBeVisible();
    }
  }

  async close(): Promise<void> {
    if (await this.popover.isVisible()) {
      await this.page.keyboard.press("Escape");
      await expect(this.popover).toBeHidden();
    }
  }

  /** Open the view-picker dropdown (no-op if already open). */
  async openViewPicker(): Promise<void> {
    if (!(await this.chipMenu.isVisible())) {
      await this.viewPicker.click();
      await expect(this.chipMenu).toBeVisible();
    }
  }

  async closeViewPicker(): Promise<void> {
    if (await this.chipMenu.isVisible()) {
      await this.page.keyboard.press("Escape");
      await expect(this.chipMenu).toBeHidden();
    }
  }

  /** Select a view by name from the picker dropdown. */
  async selectViewByName(name: string): Promise<void> {
    await this.openViewPicker();
    await this.chipByName(name).click();
    // Selecting closes the menu.
    await expect(this.chipMenu).toBeHidden();
  }

  /** Create immediately, then wait for the optional rename handoff. */
  async beginNewView(): Promise<Locator> {
    await this.openViewPicker();
    await this.newViewAction.scrollIntoViewIfNeeded();
    await this.newViewAction.click();
    await expect(this.chipMenu).toBeHidden();
    await expect(this.popover).toBeVisible();
    const input = this.popover.getByTestId("view-rename-input");
    await expect(input).toBeVisible();
    await expect(input).toBeFocused();
    return input;
  }

  /**
   * Assert the active view by name. The active view label is rendered on the
   * picker trigger button, so this works without opening the dropdown.
   */
  async expectActiveViewChip(name: string): Promise<void> {
    await this.closeViewPicker();
    await expect(this.viewPicker).toContainText(name);
  }

  /** A view chip in the (open) picker dropdown by name. */
  chipByName(name: string): Locator {
    return this.chipMenu.getByTestId("sidebar-view-chip").filter({ hasText: name }).first();
  }

  /** Read the chip names in dropdown order (opens the picker to read them). */
  async expectChipOrder(names: string[]): Promise<void> {
    await this.openViewPicker();
    await expect
      .poll(async () => {
        const texts = await this.chipMenu.getByTestId("sidebar-view-chip").allTextContents();
        return texts.map((text) => text.trim());
      })
      .toEqual(names);
    await this.closeViewPicker();
  }

  async addFilterRow(): Promise<void> {
    await this.open();
    await this.popover.getByTestId("filter-add-button").click();
  }

  clauseRow(index: number): Locator {
    return this.popover.getByTestId("filter-clause-row").nth(index);
  }

  async setClauseDimension(index: number, dimensionValue: string): Promise<void> {
    const trigger = this.clauseRow(index).getByTestId("filter-dimension-select");
    await trigger.click();
    await this.page.getByRole("option", { name: dimensionValue, exact: false }).first().click();
  }

  async setClauseOp(index: number, opLabel: string): Promise<void> {
    const trigger = this.clauseRow(index).getByTestId("filter-op-select");
    await trigger.click();
    await this.page.getByRole("option", { name: opLabel, exact: true }).first().click();
  }

  async setClauseBooleanValue(index: number, value: boolean): Promise<void> {
    const trigger = this.clauseRow(index).getByTestId("filter-value-select");
    await trigger.click();
    await this.page.getByRole("option", { name: value ? "true" : "false", exact: true }).click();
  }

  async setClauseTextValue(index: number, value: string): Promise<void> {
    await this.clauseRow(index).getByTestId("filter-value-input").fill(value);
  }

  /** Pick a single-select enum clause value (e.g. repository) by its option label. */
  async setClauseEnumValue(index: number, optionLabel: string): Promise<void> {
    const trigger = this.clauseRow(index).getByTestId("filter-value-select");
    await trigger.click();
    // No .first(): exact:true + unique slugs mean a single match, so Playwright's
    // strict mode surfaces a duplicate-option regression instead of masking it.
    await this.page.getByRole("option", { name: optionLabel, exact: true }).click();
  }

  async removeClause(index: number): Promise<void> {
    await this.clauseRow(index).getByTestId("filter-clause-remove").click();
  }

  async setSort(keyLabel: string, direction?: "asc" | "desc"): Promise<void> {
    const keyTrigger = this.popover.getByTestId("sort-key-select");
    await keyTrigger.click();
    await this.page.getByRole("option", { name: keyLabel, exact: true }).click();
    if (direction) {
      const toggle = this.popover.getByTestId("sort-direction-toggle");
      const current = (await toggle.getAttribute("data-direction")) as "asc" | "desc" | null;
      if (current && current !== direction) await toggle.click();
    }
  }

  async setGroup(groupLabel: string): Promise<void> {
    const trigger = this.popover.getByTestId("group-key-select");
    await trigger.click();
    await this.page.getByRole("option", { name: groupLabel, exact: true }).click();
  }

  async saveAs(name: string): Promise<void> {
    await this.popover.getByTestId("view-save-as-button").click();
    await this.popover.getByTestId("view-save-as-name-input").fill(name);
    await this.popover.getByTestId("view-save-as-confirm").click();
  }

  async saveOverwrite(): Promise<void> {
    await this.popover.getByTestId("view-save-button").click();
  }

  async discard(): Promise<void> {
    const discardButton = this.popover.getByTestId("view-discard-button");
    await expect(discardButton).toBeVisible();
    await discardButton.click();
  }

  async deleteActiveView(): Promise<void> {
    await this.popover.getByTestId("view-delete-button").click();
  }
}
