import { type Locator, type Page } from "@playwright/test";

export class SentrySettingsPage {
  readonly urlInput: Locator;
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly statusBanner: Locator;

  constructor(private page: Page) {
    this.urlInput = page.getByTestId("sentry-url-input");
    this.secretInput = page.getByTestId("sentry-secret-input");
    this.testButton = page.getByTestId("sentry-test-button");
    this.saveButton = page.getByTestId("sentry-save-button");
    this.deleteButton = page.getByTestId("sentry-delete-button");
    this.statusBanner = page.getByTestId("integration-auth-status-banner");
  }

  async goto() {
    await this.page.goto("/settings/integrations/sentry");
    await this.secretInput.waitFor({ state: "visible" });
  }
}
