import { expect, type Locator, type Page } from "@playwright/test";

export type TerminalThemeSnapshot = {
  background?: string;
  foreground?: string;
  cursor?: string;
  cursorAccent?: string;
  selectionBackground?: string;
  minimumContrastRatio?: number;
  [key: string]: string | number | string[] | undefined;
};

export type TerminalThemeMode = "light" | "dark";

const terminalThemeContracts: Record<TerminalThemeMode, TerminalThemeSnapshot> = {
  light: {
    background: "oklch(100% 0 0)",
    foreground: "oklch(14.5% 0 0)",
    cursor: "oklch(14.5% 0 0)",
    cursorAccent: "oklch(100% 0 0)",
    selectionBackground: "oklch(97% 0 0)",
    minimumContrastRatio: 4.5,
    black: "#000000",
    red: "#cd3131",
    green: "#008000",
    yellow: "#795e00",
    blue: "#0451a5",
    magenta: "#bc05bc",
    cyan: "#005a5a",
    white: "#555555",
    brightBlack: "#666666",
    brightRed: "#c41a16",
    brightGreen: "#006400",
    brightYellow: "#9c6500",
    brightBlue: "#1769aa",
    brightMagenta: "#9c00a8",
    brightCyan: "#007a7a",
    brightWhite: "#000000",
  },
  dark: {
    background: "#181818",
    foreground: "#d4d4d4",
    cursor: "#d4d4d4",
    cursorAccent: "#181818",
    selectionBackground: "#2a2a2a",
    minimumContrastRatio: 4.5,
    black: "#1e1e1e",
    red: "#f44747",
    green: "#6a9955",
    yellow: "#dcdcaa",
    blue: "#569cd6",
    magenta: "#c586c0",
    cyan: "#4ec9b0",
    white: "#d4d4d4",
    brightBlack: "#808080",
    brightRed: "#f44747",
    brightGreen: "#6a9955",
    brightYellow: "#dcdcaa",
    brightBlue: "#569cd6",
    brightMagenta: "#c586c0",
    brightCyan: "#4ec9b0",
    brightWhite: "#ffffff",
  },
};

export function terminalThemeContract(mode: TerminalThemeMode): TerminalThemeSnapshot {
  return terminalThemeContracts[mode];
}

export function expectTerminalTheme(
  theme: TerminalThemeSnapshot | null,
  mode: TerminalThemeMode,
  message: string,
) {
  expect(theme, `${message} should expose its complete theme snapshot`).not.toBeNull();
  expect(theme).toMatchObject(terminalThemeContract(mode));
}

export async function readTerminalHostBuffer(host: Locator): Promise<string> {
  return host.evaluate((element) => {
    type XtermHost = HTMLElement & { __xtermReadBuffer?: () => string };
    return (element as XtermHost).__xtermReadBuffer?.() ?? "";
  });
}

export async function readTerminalViewportY(host: Locator): Promise<number> {
  return host.evaluate((element) => {
    type XtermHost = HTMLElement & { __xtermReadViewportY?: () => number };
    return (element as XtermHost).__xtermReadViewportY?.() ?? -1;
  });
}

export async function readTerminalHostTheme(host: Locator): Promise<TerminalThemeSnapshot | null> {
  return host.evaluate((element) => {
    type XtermHost = HTMLElement & {
      __xtermReadTheme?: () => TerminalThemeSnapshot;
    };
    return (element as XtermHost).__xtermReadTheme?.() ?? null;
  });
}

/**
 * Close a Quick Chat terminal through the control exposed for the active
 * pointer precision. Coarse-pointer tabs intentionally replace the direct
 * close button with the 44px actions trigger, so tests must cover that path.
 */
export async function closeQuickTerminalTab(page: Page, tab: Locator): Promise<void> {
  const sequence = await tab.getAttribute("data-terminal-sequence");
  if (!sequence) throw new Error("Quick terminal tab is missing its sequence");

  const closeButton = tab.getByRole("button", { name: `Close Terminal ${sequence}` });
  if (await closeButton.isVisible().catch(() => false)) {
    await closeButton.click();
    return;
  }

  await tab.getByRole("button", { name: `Actions for Terminal ${sequence}` }).tap();
  await page.getByRole("menuitem", { name: `Close Terminal ${sequence}`, exact: true }).tap();
}
