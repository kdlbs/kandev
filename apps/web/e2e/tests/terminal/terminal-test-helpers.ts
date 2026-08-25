import type { Locator } from "@playwright/test";

export type TerminalThemeSnapshot = {
  background?: string;
  foreground?: string;
  cursor?: string;
  cursorAccent?: string;
  selectionBackground?: string;
  minimumContrastRatio?: number;
  [key: string]: string | number | string[] | undefined;
};

export async function readTerminalHostBuffer(host: Locator): Promise<string> {
  return host.evaluate((element) => {
    type XtermHost = HTMLElement & { __xtermReadBuffer?: () => string };
    return (element as XtermHost).__xtermReadBuffer?.() ?? "";
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
