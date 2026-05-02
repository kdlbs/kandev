import { type Locator, type Page } from "@playwright/test";

export class LinearSettingsPage {
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly statusBanner: Locator;

  constructor(private page: Page) {
    this.secretInput = page.getByTestId("linear-secret-input");
    this.testButton = page.getByTestId("linear-test-button");
    this.saveButton = page.getByTestId("linear-save-button");
    this.deleteButton = page.getByTestId("linear-delete-button");
    this.statusBanner = page.getByTestId("integration-auth-status-banner");
  }

  async goto() {
    await this.page.goto(`/settings/integrations/linear`);
    await this.secretInput.waitFor({ state: "visible" });
  }
}
