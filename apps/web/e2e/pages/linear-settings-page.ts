import { type Locator, type Page } from "@playwright/test";

export class LinearSettingsPage {
  readonly secretInput: Locator;
  readonly testButton: Locator;
  readonly saveButton: Locator;
  readonly deleteButton: Locator;
  readonly statusBanner: Locator;
  readonly switcher: Locator;
  readonly workspaceTrigger: Locator;
  readonly copyConfigTrigger: Locator;
  readonly copyConfigTarget: Locator;
  readonly copyConfigConfirm: Locator;

  constructor(private page: Page) {
    this.secretInput = page.getByTestId("linear-secret-input");
    this.testButton = page.getByTestId("linear-test-button");
    this.saveButton = page.getByTestId("linear-save-button");
    this.deleteButton = page.getByTestId("linear-delete-button");
    this.statusBanner = page.getByTestId("integration-auth-status-banner");
    this.switcher = page.getByTestId("integration-workspace-switcher");
    this.workspaceTrigger = this.switcher.getByTitle("Switch Workspace");
    this.copyConfigTrigger = page.getByTestId("integration-copy-config-trigger");
    this.copyConfigTarget = page.getByTestId("integration-copy-config-target");
    this.copyConfigConfirm = page.getByTestId("integration-copy-config-confirm");
  }

  async goto(query = "") {
    await this.page.goto(`/settings/integrations/linear${query}`);
    await this.secretInput.waitFor({ state: "visible" });
  }
}
