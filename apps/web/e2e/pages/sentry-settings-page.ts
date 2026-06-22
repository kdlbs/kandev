import { type Locator, type Page } from "@playwright/test";

// SentrySettingsPage drives the multi-instance Sentry settings UI: a list of
// instance cards (each with its own auth-health banner + edit/delete) plus an
// add/edit form revealed by "Add instance".
export class SentrySettingsPage {
  readonly addInstanceButton: Locator;
  readonly nameInput: Locator;
  readonly urlInput: Locator;
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly cancelButton: Locator;
  readonly emptyState: Locator;

  constructor(private page: Page) {
    this.addInstanceButton = page.getByTestId("sentry-add-instance-button");
    this.nameInput = page.getByTestId("sentry-name-input");
    this.urlInput = page.getByTestId("sentry-url-input");
    this.secretInput = page.getByTestId("sentry-secret-input");
    this.testButton = page.getByTestId("sentry-test-button");
    this.saveButton = page.getByTestId("sentry-save-button");
    this.cancelButton = page.getByTestId("sentry-cancel-button");
    this.emptyState = page.getByTestId("sentry-no-instances");
  }

  async goto() {
    await this.page.goto("/settings/integrations/sentry");
    await this.addInstanceButton.waitFor({ state: "visible" });
  }

  // openAddForm reveals the empty add-instance form.
  async openAddForm() {
    await this.addInstanceButton.click();
    await this.nameInput.waitFor({ state: "visible" });
  }

  // addInstance fills and saves a new instance, then waits for its card.
  async addInstance(opts: { name: string; url?: string; secret: string }) {
    await this.openAddForm();
    await this.nameInput.fill(opts.name);
    if (opts.url) await this.urlInput.fill(opts.url);
    await this.secretInput.fill(opts.secret);
    await this.saveButton.click();
    await this.card(opts.name).waitFor({ state: "visible" });
  }

  // card scopes to a single instance card by its rendered name.
  card(name: string): Locator {
    return this.page.locator(`[data-testid="sentry-instance-card"][data-instance-name="${name}"]`);
  }

  statusBanner(name: string): Locator {
    return this.card(name).getByTestId("integration-auth-status-banner");
  }

  deleteButton(name: string): Locator {
    return this.card(name).getByTestId("sentry-instance-delete");
  }
}
