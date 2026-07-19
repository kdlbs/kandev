import { expect, type Locator, type Page } from "@playwright/test";

export class LayoutSettingsPage {
  readonly root: Locator;
  readonly editor: Locator;
  readonly toolbar: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("layout-settings");
    this.editor = page.getByTestId("layout-editor");
    this.toolbar = page.getByTestId("layout-editor-toolbar");
  }

  async openFromMobileMenu(): Promise<void> {
    await this.page.goto("/settings/general/terminal");
    await this.page.getByTestId("settings-mobile-menu-button").click();
    const menu = this.page.getByTestId("settings-mobile-menu");
    await expect(menu).toBeVisible();
    await menu.getByRole("link", { name: "Layouts", exact: true }).click();
    await expect(this.page).toHaveURL(/\/settings\/general\/layouts$/);
    await expect(this.root).toBeVisible();
  }

  async duplicateDefault(name: string): Promise<void> {
    await this.page.getByTestId("layout-profile-built-in-default").click();
    await this.page.getByTestId("layout-profile-duplicate").click();
    const nameInput = this.page.getByRole("textbox", { name: "Layout profile name" });
    await expect(nameInput).toBeVisible();
    await nameInput.fill(name);
    await expect(this.toolbar.getByRole("combobox", { name: "Selected panel" })).toBeEnabled();
  }

  async selectPanel(name: string): Promise<void> {
    await this.toolbar.getByRole("combobox", { name: "Selected panel" }).click();
    await this.page.getByRole("option", { name, exact: true }).click();
  }

  async moveSelectedTabRight(): Promise<void> {
    const button = this.toolbar.getByRole("button", { name: "Move tab right" });
    await expect(button).toBeEnabled();
    await button.click();
  }

  async save(): Promise<void> {
    const response = this.page.waitForResponse(
      (candidate) =>
        candidate.url().includes("/api/v1/user/settings") &&
        candidate.request().method() === "PATCH",
    );
    await this.root.getByRole("button", { name: "Save", exact: true }).click();
    expect((await response).ok()).toBe(true);
  }
}
