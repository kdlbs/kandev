import type { Locator, Page } from "@playwright/test";

/** Focused page object for agent-profile controls in the New Session dialog. */
export class NewSessionDialogPage {
  constructor(private readonly page: Page) {}

  /** Agent profile combobox inside the new-session or handoff dialog. */
  agentSelector(): Locator {
    return this.page
      .getByRole("dialog")
      .filter({ hasText: /New agent in|Hand off to/ })
      .getByTestId("agent-profile-selector");
  }

  /** Visible agent profile options for the active new-session selector. */
  agentOptions(): Locator {
    return this.page.getByRole("listbox", { name: "Suggestions" }).getByRole("option");
  }

  /** Select an agent profile from the active new-session or handoff dialog. */
  async selectProfile(profileName: string, touch = false): Promise<void> {
    const selector = this.agentSelector();
    if (touch) {
      await selector.tap();
    } else {
      await selector.click();
    }
    const option = this.agentOptions().filter({ hasText: profileName });
    if (touch) {
      await option.tap();
    } else {
      await option.click();
    }
  }
}
