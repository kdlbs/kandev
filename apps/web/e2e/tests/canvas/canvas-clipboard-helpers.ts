import { expect, type Locator, type Page } from "@playwright/test";

export async function verifyRegistryExampleCopy(
  page: Page,
  dialog: Locator,
  rejectModern: boolean,
  touch: boolean,
): Promise<void> {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  const help = dialog
    .locator("details")
    .filter({ has: page.getByText("How to share", { exact: true }) });
  await expect(help).toHaveCount(1);
  const summary = help.getByText("How to share", { exact: true });
  if (touch) await summary.tap();
  else await summary.click();
  const example = await help.locator("pre").textContent();
  expect(example).toBeTruthy();
  await page.evaluate(() => navigator.clipboard.writeText("clipboard sentinel"));
  if (rejectModern) {
    await page.evaluate(() => {
      let attempts = 0;
      Object.defineProperty(navigator.clipboard, "__testRejectedWrites", {
        configurable: true,
        get: () => attempts,
      });
      Object.defineProperty(navigator.clipboard, "writeText", {
        configurable: true,
        value: async () => {
          attempts += 1;
          throw new DOMException("Controlled rejection", "NotAllowedError");
        },
      });
    });
  }
  try {
    const copy = help.getByRole("button", { name: "Copy registry example", exact: true });
    if (touch) await copy.tap();
    else await copy.click();
    await expect(
      help.getByRole("button", { name: "Registry example copied", exact: true }),
    ).toBeVisible();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(example);
    if (rejectModern) {
      expect(
        await page.evaluate(() => Reflect.get(navigator.clipboard, "__testRejectedWrites")),
      ).toBe(1);
    }
  } finally {
    if (rejectModern) {
      await page.evaluate(() => {
        Reflect.deleteProperty(navigator.clipboard, "writeText");
        Reflect.deleteProperty(navigator.clipboard, "__testRejectedWrites");
      });
    }
  }
}
