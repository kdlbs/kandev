import { expect, test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { readTerminalHostBuffer, readTerminalHostTheme } from "./terminal-test-helpers";

async function activeTerminalHost(testPage: Parameters<typeof seedIdleSession>[0]) {
  return testPage
    .getByTestId("terminal-panel")
    .locator('[data-testid="terminal-xterm-host"]:visible')
    .first();
}

test.describe("adaptive terminal themes", () => {
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

    const initialTheme = await readTerminalHostTheme(host);
    expect(initialTheme, "the active xterm should expose its theme snapshot").not.toBeNull();
    expect(initialTheme?.minimumContrastRatio).toBe(4.5);
    const initialBuffer = await readTerminalHostBuffer(host);

    const themeToggle = testPage.getByRole("button", { name: "Switch to Dark Mode" }).first();
    await expect(themeToggle).toBeVisible();
    await themeToggle.click();
    await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);

    await expect
      .poll(() => readTerminalHostTheme(host), {
        timeout: 5_000,
        message: "the open terminal should receive the resolved dark theme",
      })
      .not.toEqual(initialTheme);
    await expect
      .poll(() => readTerminalHostBuffer(host), {
        timeout: 5_000,
        message: "the theme update should preserve the terminal buffer",
      })
      .toContain(beforeMarker);

    const afterMarker = "TERMINAL_THEME_AFTER";
    await session.typeInTerminal(`printf ${afterMarker}`);
    await session.expectTerminalHasText(afterMarker);
    expect(await readTerminalHostBuffer(host)).toContain(initialBuffer);
    await prCapture.screenshot("terminal-theme-dark", {
      caption: "Open task terminal after switching from the light theme to the dark theme",
    });
  });
});
