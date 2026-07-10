import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Page Object for the SSH executor settings UI. Wraps the two cards
 * (SSHConnectionCard + SSHSessionsCard) in a single ergonomic surface so
 * specs read like user actions, not test-id soup.
 *
 * All locators bind to data-testid attributes added in the SSH UI commit.
 * If you touch a testid in the components, update the matching getter here
 * in the same PR.
 */
export class SSHSettingsPage {
  constructor(private readonly page: Page) {}

  /** Navigate to the new-executor flow. */
  async gotoNew(): Promise<void> {
    await this.page.goto("/settings/executors/new/ssh");
    await expect(this.connectionCard).toBeVisible();
  }

  /** Navigate to the edit page for an existing SSH executor. */
  async gotoExisting(executorId: string): Promise<void> {
    await this.page.goto(`/settings/executors/ssh/${executorId}`);
    await expect(this.connectionCard).toBeVisible();
  }

  // --- card roots ---

  get connectionCard(): Locator {
    return this.page.getByTestId("ssh-connection-card");
  }

  get sessionsCard(): Locator {
    return this.page.getByTestId("ssh-sessions-card");
  }

  get connectionBadge(): Locator {
    return this.page.getByTestId("ssh-connection-badge");
  }

  // --- form fields ---

  get nameInput(): Locator {
    return this.page.getByTestId("ssh-input-name");
  }

  get hostInput(): Locator {
    return this.page.getByTestId("ssh-input-host");
  }

  get hostAliasInput(): Locator {
    return this.page.getByTestId("ssh-input-host-alias");
  }

  get portInput(): Locator {
    return this.page.getByTestId("ssh-input-port");
  }

  get userInput(): Locator {
    return this.page.getByTestId("ssh-input-user");
  }

  get identitySourceTrigger(): Locator {
    return this.page.getByTestId("ssh-input-identity-source");
  }

  get identityFileInput(): Locator {
    return this.page.getByTestId("ssh-input-identity-file");
  }

  get proxyJumpInput(): Locator {
    return this.page.getByTestId("ssh-input-proxy-jump");
  }

  /**
   * Fill the connection form with the supplied subset of fields. Skips
   * fields that aren't passed so tests can apply partial updates without
   * blanking the rest of the form.
   */
  async fillForm(opts: {
    name?: string;
    host?: string;
    hostAlias?: string;
    port?: number;
    user?: string;
    identitySource?: "agent" | "file";
    identityFile?: string;
    proxyJump?: string;
  }): Promise<void> {
    if (opts.name !== undefined) await this.nameInput.fill(opts.name);
    if (opts.hostAlias !== undefined) await this.hostAliasInput.fill(opts.hostAlias);
    if (opts.host !== undefined) await this.hostInput.fill(opts.host);
    if (opts.port !== undefined) await this.portInput.fill(String(opts.port));
    if (opts.user !== undefined) await this.userInput.fill(opts.user);
    if (opts.identitySource !== undefined) {
      await this.identitySourceTrigger.click();
      await this.page.getByTestId(`ssh-input-identity-source-${opts.identitySource}`).click();
    }
    if (opts.identityFile !== undefined) await this.identityFileInput.fill(opts.identityFile);
    if (opts.proxyJump !== undefined) await this.proxyJumpInput.fill(opts.proxyJump);
  }

  // --- actions ---

  get testButton(): Locator {
    return this.page.getByTestId("ssh-test-button");
  }

  get saveButton(): Locator {
    return this.page.getByTestId("ssh-save-button");
  }

  get trustCheckbox(): Locator {
    return this.page.getByTestId("ssh-trust-checkbox");
  }

  async clickTest(): Promise<void> {
    await this.testButton.click();
  }

  async clickSave(): Promise<void> {
    await this.saveButton.click();
  }

  async tickTrust(): Promise<void> {
    await this.trustCheckbox.check();
  }

  async untickTrust(): Promise<void> {
    await this.trustCheckbox.uncheck();
  }

  /**
   * Wait for the test result to render and return its data-success
   * attribute as "true" | "false" so tests can branch on the outcome.
   */
  async waitForTestResult(): Promise<"true" | "false"> {
    const result = this.page.getByTestId("ssh-test-result");
    await expect(result).toBeVisible({ timeout: 30_000 });
    const success = await result.getAttribute("data-success");
    return (success ?? "false") as "true" | "false";
  }

  // --- assertions ---

  /** The full SHA256:... fingerprint reported by the most recent test run. */
  observedFingerprint(): Locator {
    return this.page.getByTestId("ssh-fingerprint-observed");
  }

  pinnedFingerprint(): Locator {
    return this.page.getByTestId("ssh-fingerprint-pinned-value");
  }

  fingerprintChangeWarning(): Locator {
    return this.page.getByTestId("ssh-fingerprint-change-warning");
  }

  /** Per-step row by slugified step name (e.g. "ssh-handshake", "probe-remote"). */
  step(slug: string): Locator {
    return this.page.getByTestId(`ssh-test-step-${slug}`);
  }

  async expectStepSuccess(slug: string): Promise<void> {
    await expect(this.step(slug)).toHaveAttribute("data-success", "true");
  }

  async expectStepFailure(slug: string): Promise<void> {
    await expect(this.step(slug)).toHaveAttribute("data-success", "false");
  }

  // --- sessions card ---

  get sessionsEmpty(): Locator {
    return this.page.getByTestId("ssh-sessions-empty");
  }

  get sessionsTable(): Locator {
    return this.page.getByTestId("ssh-sessions-table");
  }

  get sessionsRefresh(): Locator {
    return this.page.getByTestId("ssh-sessions-refresh");
  }

  sessionRow(sessionId: string): Locator {
    return this.page.getByTestId(`ssh-session-row-${sessionId}`);
  }
}
