import type { Locator } from "@playwright/test";

export async function readTerminalHostBuffer(host: Locator): Promise<string> {
  return host.evaluate((element) => {
    type XtermHost = HTMLElement & { __xtermReadBuffer?: () => string };
    return (element as XtermHost).__xtermReadBuffer?.() ?? "";
  });
}
