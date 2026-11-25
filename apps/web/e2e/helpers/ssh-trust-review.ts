import { expect, test, type Page } from "@playwright/test";
import { seedSSHLayout } from "./ssh-management-layout";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.16, 004.17, 004.19
export async function exerciseSSHTrustReview(page: Page, baseUrl: string, touch: boolean) {
  const { route } = await seedSSHLayout(baseUrl);
  const oldPin = "SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  const newPin = "SHA256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
  route.destination.fingerprint = oldPin;
  await page.route("**/api/v1/ssh/managed/connections", (r) => r.fulfill({ json: [route] }));
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/connections/${route.destination.id}/observe`, async (r) => {
    await pending;
    await r.fulfill({
      json: {
        stamp: route.stamp,
        receipt: "ui-review-receipt",
        fingerprint: newPin,
        public_key: "public",
      },
    });
  });
  let trusts = 0;
  await page.route(`**/connections/${route.destination.id}/trust`, async (r) => {
    trusts++;
    expect(r.request().postDataJSON()).toMatchObject({
      expected_stamp: route.stamp,
      receipt: "ui-review-receipt",
      fingerprint: newPin,
    });
    route.destination.fingerprint = newPin;
    route.stamp = `${route.stamp}-trusted`;
    await r.fulfill({ json: route });
  });
  await page.goto(`/settings/executors/ssh-connections?id=${route.destination.id}&section=access`);
  const inspect = page.getByRole("button", { name: "Inspect host key", exact: true });
  const details = page.getByRole("button", { name: "Host details", exact: true });
  if (touch) {
    await details.tap();
    await inspect.tap();
  } else {
    await details.click();
    await inspect.click();
  }
  try {
    await expect(page.getByRole("status")).toContainText("Reading the server fingerprint");
  } finally {
    release();
  }
  const heading = page.getByRole("heading", { name: "Verify this server", exact: true });
  await expect(heading).toBeFocused();
  await expect(page.getByText("The server identity has changed", { exact: true })).toBeVisible();
  await expect(page.getByText(oldPin, { exact: true })).toBeVisible();
  await expect(page.getByText(newPin, { exact: true })).toBeVisible();
  const confirm = page.getByRole("button", { name: "Trust this host key", exact: true });
  const box = await confirm.boundingBox();
  expect(box).not.toBeNull();
  if (touch) expect(box!.height).toBeGreaterThanOrEqual(44);
  else expect(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
  await assertNoDocumentHorizontalOverflow(page);
  for (const theme of ["light", "dark"]) {
    await page.emulateMedia({ colorScheme: theme as "light" | "dark" });
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await confirm.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: test.info().outputPath(`ssh-trust-review-${theme}.png`),
      fullPage: true,
    });
  }
  expect(trusts).toBe(0);
  if (touch) await confirm.tap();
  else await confirm.click();
  await expect(inspect).toBeFocused();
  expect(trusts).toBe(1);
  await expect(page.getByRole("dialog")).toHaveCount(0);
}
