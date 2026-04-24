// Terminal panel search — xterm addon-search integration.
import { test, expect } from "../../fixtures/test-base";
import {
  openPanelSearch,
  closePanelSearch,
  panelSearchBar,
  panelSearchInput,
  panelSearchMatchCounter,
  panelSearchToggle,
} from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription, MODIFIER } from "./shared";

/** Wait until the terminal xterm buffer contains the given text. */
async function waitForTerminalText(
  page: import("@playwright/test").Page,
  text: string,
  timeout = 15_000,
): Promise<void> {
  await expect
    .poll(
      async () =>
        page.evaluate((needle: string) => {
          const panel = document.querySelector('[data-testid="terminal-panel"]');
          const xtermEl = panel?.querySelector(".xterm");
          type XC = HTMLElement & { __xtermReadBuffer?: () => string };
          const container = xtermEl?.parentElement as XC | null | undefined;
          return (container?.__xtermReadBuffer?.() ?? "").includes(needle);
        }, text),
      { timeout, message: `Waiting for terminal to contain "${text}"` },
    )
    .toBe(true);
}

/** Seed terminal content: types a loop producing many "hello world N" lines. */
async function seedTerminalOutput(page: import("@playwright/test").Page): Promise<void> {
  const session = page.getByTestId("terminal-panel").locator(".xterm");
  await session.click();
  // Wait for shell prompt to render (buffer non-empty)
  await expect
    .poll(
      async () =>
        page.evaluate(() => {
          const panel = document.querySelector('[data-testid="terminal-panel"]');
          const xtermEl = panel?.querySelector(".xterm");
          type XC = HTMLElement & { __xtermReadBuffer?: () => string };
          const container = xtermEl?.parentElement as XC | null | undefined;
          return (container?.__xtermReadBuffer?.() ?? "").length > 0;
        }),
      { timeout: 20_000, message: "Waiting for terminal shell buffer" },
    )
    .toBe(true);
  await page.keyboard.type(`for i in 1 2 3 4 5 6 7 8 9 10; do echo "hello world $i"; done`);
  await page.keyboard.press("Enter");
  await waitForTerminalText(page, "hello world 10");
}

test.describe("@search terminal panel search", () => {
  test.describe.configure({ retries: 1 });

  test("T1+T2 Ctrl+F opens bar; matching query updates counter", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await seedTask(testPage, apiClient, seedData, "terminal-search-basic", {
      description: seedMessagesDescription(["idle"]),
    });
    await seedTerminalOutput(testPage);

    const bufferBefore = await testPage.evaluate(() => {
      const panel = document.querySelector('[data-testid="terminal-panel"]');
      const xtermEl = panel?.querySelector(".xterm");
      type XC = HTMLElement & { __xtermReadBuffer?: () => string };
      const container = xtermEl?.parentElement as XC | null | undefined;
      return container?.__xtermReadBuffer?.() ?? "";
    });

    await openPanelSearch(testPage, "terminal");

    // Xterm should not have received a literal "f" — buffer is unchanged.
    const bufferAfter = await testPage.evaluate(() => {
      const panel = document.querySelector('[data-testid="terminal-panel"]');
      const xtermEl = panel?.querySelector(".xterm");
      type XC = HTMLElement & { __xtermReadBuffer?: () => string };
      const container = xtermEl?.parentElement as XC | null | undefined;
      return container?.__xtermReadBuffer?.() ?? "";
    });
    expect(bufferAfter).toBe(bufferBefore);

    await prCapture.startRecording("terminal-search");
    await panelSearchInput(testPage).fill("hello world");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
        message: "Waiting for match counter to reflect matches",
      })
      .toMatch(/^[1-9]\d* \/ [1-9]\d*$/);
    await prCapture.stopRecording({ caption: "Terminal search: incremental match highlighting" });
  });

  test("T4 case sensitivity toggle affects matches", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    await seedTask(testPage, apiClient, seedData, "terminal-search-case", {
      description: seedMessagesDescription(["idle"]),
    });
    await seedTerminalOutput(testPage);

    await openPanelSearch(testPage, "terminal");
    await panelSearchInput(testPage).fill("HELLO");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
      })
      .toMatch(/^[1-9]\d* \/ [1-9]\d*$/);

    // Enable case-sensitive — "HELLO" should no longer match lowercase output
    await panelSearchToggle(testPage, "Match case").click();
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
      })
      .toBe("0 / 0");
  });

  test("T5+T6 regex toggle: valid + invalid patterns", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedTask(testPage, apiClient, seedData, "terminal-search-regex", {
      description: seedMessagesDescription(["idle"]),
    });
    await seedTerminalOutput(testPage);

    await openPanelSearch(testPage, "terminal");
    await panelSearchToggle(testPage, "Regular expression").click();
    await panelSearchInput(testPage).fill("hello world \\d+");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
      })
      .toMatch(/^[1-9]\d* \/ [1-9]\d*$/);

    // Invalid regex surfaces an error
    await panelSearchInput(testPage).fill("[unclosed");
    await expect(panelSearchBar(testPage).getByText("Invalid regex")).toBeVisible({
      timeout: 3_000,
    });
    await expect(panelSearchInput(testPage)).toHaveAttribute("aria-invalid", "true");
  });

  test("T8 Escape inside terminal closes bar without leaking Esc to PTY", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedTask(testPage, apiClient, seedData, "terminal-search-esc", {
      description: seedMessagesDescription(["idle"]),
    });
    await seedTerminalOutput(testPage);

    // Capture buffer snapshot before opening search
    const getBuffer = () =>
      testPage.evaluate(() => {
        const panel = document.querySelector('[data-testid="terminal-panel"]');
        const xtermEl = panel?.querySelector(".xterm");
        type XC = HTMLElement & { __xtermReadBuffer?: () => string };
        const container = xtermEl?.parentElement as XC | null | undefined;
        return container?.__xtermReadBuffer?.() ?? "";
      });
    await openPanelSearch(testPage, "terminal");
    const beforeEsc = await getBuffer();
    await closePanelSearch(testPage);
    const afterEsc = await getBuffer();
    // Buffer should be identical — Esc did not produce any terminal output
    expect(afterEsc).toBe(beforeEsc);
  });

  test("T11 Backspace in query adjusts match counter", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedTask(testPage, apiClient, seedData, "terminal-search-backspace", {
      description: seedMessagesDescription(["idle"]),
    });
    await seedTerminalOutput(testPage);

    await openPanelSearch(testPage, "terminal");
    await panelSearchInput(testPage).fill("hello world zzz");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
      })
      .toBe("0 / 0");
    // Remove the trailing " zzz" so query becomes "hello world" and matches appear
    await panelSearchInput(testPage).fill("hello world");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 8_000,
      })
      .toMatch(/^[1-9]\d* \/ [1-9]\d*$/);
  });

  void MODIFIER;
});
