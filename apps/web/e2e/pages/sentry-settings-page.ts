import { type Locator, type Page } from "@playwright/test";

// SentrySettingsPage drives the multi-instance Sentry settings UI: an instance
// list of cards plus mutually-exclusive add/edit forms.
export class SentrySettingsPage {
  readonly addInstanceButton: Locator;
  readonly cards: Locator;
  readonly noInstances: Locator;
  readonly instanceListLoading: Locator;
  readonly addNameInput: Locator;
  readonly addUrlInput: Locator;
  readonly addSecretInput: Locator;
  readonly addTestButton: Locator;
  readonly addSaveButton: Locator;

  constructor(private page: Page) {
    this.addInstanceButton = page.getByTestId("sentry-add-instance-button");
    this.cards = page.getByTestId("sentry-instance-card");
    this.noInstances = page.getByTestId("sentry-no-instances");
    this.instanceListLoading = page.getByTestId("sentry-instance-list-loading");
    this.addNameInput = page.getByTestId("sentry-add-name-input");
    this.addUrlInput = page.getByTestId("sentry-add-url-input");
    this.addSecretInput = page.getByTestId("sentry-add-secret-input");
    this.addTestButton = page.getByTestId("sentry-add-test-button");
    this.addSaveButton = page.getByRole("button", { name: "Save changes" });
  }

  async goto(workspaceId: string) {
    await this.page.goto(`/settings/workspaces/${workspaceId}/integrations/sentry`);
    // The "Add instance" button renders whenever no form is open — present in
    // both the empty and populated states. It is not sufficient on its own:
    // it also renders while the async instance list is loading.
    await this.addInstanceButton.waitFor({ state: "visible" });
    await this.instanceListLoading.waitFor({ state: "hidden", timeout: 15_000 });
  }

  async addInstance(name: string, secret: string, url?: string) {
    await this.addInstanceButton.click();
    await this.addNameInput.fill(name);
    if (url) await this.addUrlInput.fill(url);
    await this.addSecretInput.fill(secret);
    await this.addSaveButton.click();
  }

  // cardByName scopes to the card whose visible name matches, so per-instance
  // assertions (banner, delete) target the right card.
  cardByName(name: string): Locator {
    return this.cards.filter({ hasText: name });
  }
}
