import { type Locator, type Page } from "@playwright/test";

export class SentrySettingsPage {
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly statusBanner: Locator;
  readonly orgSelect: Locator;
  readonly projectSelect: Locator;

  constructor(private page: Page) {
    this.secretInput = page.getByTestId("sentry-secret-input");
    this.testButton = page.getByTestId("sentry-test-button");
    this.saveButton = page.getByTestId("sentry-save-button");
    this.deleteButton = page.getByTestId("sentry-delete-button");
    this.statusBanner = page.getByTestId("integration-auth-status-banner");
    this.orgSelect = page.getByTestId("sentry-org-input");
    // The project Select trigger has no testid; it is reachable by its id.
    this.projectSelect = page.locator("#sentry-project");
  }

  async goto() {
    await this.page.goto("/settings/integrations/sentry");
    await this.secretInput.waitFor({ state: "visible" });
  }
}
