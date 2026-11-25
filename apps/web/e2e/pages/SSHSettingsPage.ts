import { expect, type Locator, type Page } from "@playwright/test";

/** Shared binding, reachability, and sessions on the SSH executor settings page. */
export class SSHSettingsPage {
  constructor(private readonly page: Page) {}

  /** Navigate to the new-executor flow. */
  async gotoNew(): Promise<void> {
    await this.page.goto("/settings/executors/new/ssh");
    await expect(this.connectionPicker).toBeVisible();
  }

  /** Navigate to the edit page for an existing SSH executor. */
  async gotoExisting(executorId: string): Promise<void> {
    await this.page.goto(`/settings/executors/ssh/${executorId}`);
    await expect(this.connectionPicker).toBeVisible();
    await expect(this.sessionsCard).toBeVisible({ timeout: 15_000 });
    await expect(this.sessionsEmpty.or(this.sessionsTable)).toBeVisible({ timeout: 15_000 });
  }

  get connectionPicker(): Locator {
    return this.page.getByLabel("Shared connection", { exact: true });
  }

  get sessionsCard(): Locator {
    return this.page.getByTestId("ssh-sessions-card");
  }

  get nameInput(): Locator {
    return this.page.getByLabel("Name", { exact: true });
  }

  get testButton(): Locator {
    return this.page.getByRole("button", { name: "Test executor", exact: true });
  }

  get saveButton(): Locator {
    return this.page.getByRole("button", { name: "Save", exact: true });
  }

  get floatingSaveButton(): Locator {
    return this.page.getByRole("button", { name: "Save changes", exact: true });
  }

  get executorReady(): Locator {
    return this.page
      .getByRole("status")
      .filter({ hasText: "SSH login succeeded and the remote platform is supported." });
  }

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

  // --- reachability card ---

  get reachabilityCard(): Locator {
    return this.page.getByTestId("ssh-reachability-card");
  }

  get reachabilityState(): Locator {
    return this.page.getByTestId("ssh-reachability-state");
  }

  get reachabilityHost(): Locator {
    return this.page.getByTestId("ssh-reachability-host");
  }

  get reachabilityReason(): Locator {
    return this.page.getByTestId("ssh-reachability-reason");
  }

  get reachabilityProbeNowButton(): Locator {
    return this.page.getByTestId("ssh-reachability-probe-now");
  }

  /**
   * Click "Probe now" and wait for the synchronous probe route to return.
   * The button re-enabling is the completion signal — the click handler
   * awaits the request before clearing its own disabled state.
   */
  async probeNow(): Promise<void> {
    await this.reachabilityProbeNowButton.click();
    await expect(this.reachabilityProbeNowButton).toBeEnabled({ timeout: 30_000 });
  }
}
