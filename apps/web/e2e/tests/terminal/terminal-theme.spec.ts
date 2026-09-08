import { expect, test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import {
  expectTerminalTheme,
  readTerminalHostBuffer,
  readTerminalHostTheme,
  readTerminalViewportY,
  terminalThemeContract,
} from "./terminal-test-helpers";

async function activeTerminalHost(testPage: Parameters<typeof seedIdleSession>[0]) {
  return testPage
    .getByTestId("terminal-panel")
    .locator('[data-testid="terminal-xterm-host"]:visible');
}

test.describe("adaptive terminal themes", () => {
  test("constructs an initial terminal with the saved dark theme", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.addInitScript(() => localStorage.setItem("theme", "dark"));
    await seedIdleSession(testPage, apiClient, seedData, "Initial dark terminal theme");

    const host = await activeTerminalHost(testPage);
    await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);
    const theme = await readTerminalHostTheme(host);

    expectTerminalTheme(theme, "dark", "the initial xterm");
  });

  test("updates an open terminal when the application theme changes", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Terminal theme synchronization",
    );
    const host = await activeTerminalHost(testPage);
    const beforeMarker = "TERMINAL_THEME_BEFORE";
    await session.typeInTerminal(`printf ${beforeMarker}`);
    await session.expectTerminalHasText(beforeMarker);

    const xterm = host.locator(".xterm");
    const helperTextarea = xterm.locator(".xterm-helper-textarea");
    await xterm.click();
    await expect(helperTextarea).toBeFocused();

    await session.typeInTerminal("seq 1 500");
    await expect.poll(() => readTerminalHostBuffer(host)).toContain("500");
    const bottomViewportY = await readTerminalViewportY(host);
    expect(bottomViewportY).toBeGreaterThan(0);
    const runningMarker = "TERMINAL_THEME_RUNNING";
    await testPage.keyboard.type(`sleep 10; printf ${runningMarker}`);
    await testPage.keyboard.press("Enter");

    await xterm.click();
    await xterm.hover();
    await testPage.mouse.wheel(0, -1200);
    await expect
      .poll(() => readTerminalViewportY(host), {
        message: "Mouse wheel should scroll the terminal into its scrollback",
      })
      .toBeLessThan(bottomViewportY);

    const viewportBeforeTheme = await readTerminalViewportY(host);

    const initialTheme = await readTerminalHostTheme(host);
    expectTerminalTheme(initialTheme, "light", "the active xterm before switching");
    const initialBuffer = await readTerminalHostBuffer(host);

    const themeToggle = testPage.getByRole("button", {
      name: "Switch to Dark Mode",
      exact: true,
    });
    await expect(themeToggle).toBeVisible();
    await themeToggle.evaluate((element) => (element as HTMLButtonElement).click());
    await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);

    await expect
      .poll(() => readTerminalHostTheme(host), {
        timeout: 5_000,
        message: "the open terminal should receive the resolved dark theme",
      })
      .toMatchObject(terminalThemeContract("dark"));
    expectTerminalTheme(
      await readTerminalHostTheme(host),
      "dark",
      "the active xterm after switching",
    );
    await expect
      .poll(() => readTerminalHostBuffer(host), {
        timeout: 5_000,
        message: "the theme update should preserve the terminal buffer",
      })
      .toContain(beforeMarker);
    await expect(helperTextarea).toBeFocused();
    const viewportAfterTheme = await readTerminalViewportY(host);
    expect(Math.abs(viewportAfterTheme - viewportBeforeTheme)).toBeLessThanOrEqual(1);

    const afterMarker = "TERMINAL_THEME_AFTER";
    await expect
      .poll(() => readTerminalHostBuffer(host), { timeout: 15_000 })
      .toContain(runningMarker);
    await session.typeInTerminal(`printf ${afterMarker}`);
    await session.expectTerminalHasText(afterMarker);
    expect(await readTerminalHostBuffer(host)).toContain(initialBuffer);
    await prCapture.screenshot("terminal-theme-dark", {
      caption: "Open task terminal after switching from the light theme to the dark theme",
    });
  });
});
