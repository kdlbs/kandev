import { type Locator, type Page, expect } from "@playwright/test";

export class SidebarFilterPopoverPage {
  readonly bar: Locator;
  readonly gear: Locator;
  readonly chipRow: Locator;

  constructor(private readonly page: Page) {
    this.bar = page.getByTestId("sidebar-filter-bar");
    this.gear = page.getByTestId("sidebar-filter-gear");
    this.chipRow = page.getByTestId("sidebar-view-chip-row");
  }

  get popover(): Locator {
    return this.page.getByTestId("sidebar-filter-popover");
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

  async selectViewByName(name: string): Promise<void> {
    await this.chipByName(name).click();
  }

  async expectActiveViewChip(name: string): Promise<void> {
    const chip = this.chipRow
      .locator(`[data-testid='sidebar-view-chip'][data-active='true']`)
      .first();
    await expect(chip).toContainText(name);
  }

  chipByName(name: string): Locator {
    return this.chipRow.getByTestId("sidebar-view-chip").filter({ hasText: name }).first();
  }

  async expectChipOrder(names: string[]): Promise<void> {
    await expect
      .poll(async () => {
        const texts = await this.chipRow.getByTestId("sidebar-view-chip").allTextContents();
        return texts.map((text) => text.trim());
      })
      .toEqual(names);
  }

  async dragViewBefore(sourceName: string, targetName: string): Promise<void> {
    const source = this.chipByName(sourceName);
    const target = this.chipByName(targetName);
    await source.scrollIntoViewIfNeeded();
    await target.scrollIntoViewIfNeeded();
    await expect(source).toBeVisible();

    const sourceBox = await source.boundingBox();
    const targetBox = await target.boundingBox();
    if (!sourceBox || !targetBox) throw new Error("Missing sidebar view chip drag geometry");
    const targetLeftHalfX = targetBox.x + targetBox.width * 0.25;

    await this.page.mouse.move(
      sourceBox.x + sourceBox.width / 2,
      sourceBox.y + sourceBox.height / 2,
    );
    await this.page.mouse.down();
    // Move far enough to exceed the 8px PointerSensor activation threshold,
    // then slide toward the target.
    await this.page.mouse.move(
      sourceBox.x + sourceBox.width / 2 - 16,
      sourceBox.y + sourceBox.height / 2,
      { steps: 4 },
    );
    await this.page.mouse.move(targetLeftHalfX, targetBox.y + targetBox.height / 2, {
      steps: 20,
    });
    await this.page.mouse.up();
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
    await this.popover.getByTestId("view-discard-button").click();
  }

  async deleteActiveView(): Promise<void> {
    await this.popover.getByTestId("view-delete-button").click();
  }
}
