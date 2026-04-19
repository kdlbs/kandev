import type { Page, Locator } from "@playwright/test";

export class MobileTerminalKeybarPage {
  readonly root: Locator;
  readonly ctrl: Locator;
  readonly ctrlC: Locator;
  readonly ctrlD: Locator;

  constructor(private readonly page: Page) {
    this.root = page.getByTestId("mobile-terminal-keybar");
    this.ctrl = page.getByTestId("keybar-key-ctrl");
    this.ctrlC = page.getByTestId("keybar-key-ctrl-c");
    this.ctrlD = page.getByTestId("keybar-key-ctrl-d");
  }

  key(id: string): Locator {
    return this.page.getByTestId(`keybar-key-${id}`);
  }

  async tap(id: string): Promise<void> {
    await this.key(id).tap();
  }
}
