import { expect, test, type Page } from "@playwright/test";
import { seedSSHLayout } from "./ssh-management-layout";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
export async function exerciseSSHDiagnostics(page: Page, baseUrl: string, touch: boolean) {
  const { route } = await seedSSHLayout(baseUrl);
  route.destination.fingerprint = "SHA256:public-fixture-host";
  await page.route("**/api/v1/ssh/managed/connections", (r) => r.fulfill({ json: [route] }));
  let response = { stamp: route.stamp, ready: true, checks: [] as { code: string; hop: string }[] };
  let fail = false;
  await page.route(`**/connections/${route.destination.id}/preflight`, (r) =>
    r.fulfill(fail ? { status: 503, json: { error: "ssh_operation_failed" } } : { json: response }),
  );
  await page.route(`**/connections/${route.destination.id}/test`, (r) =>
    r.fulfill({ status: 400, json: { error: "ssh_authentication_rejected", hop: "destination" } }),
  );
  await page.goto(
    `/settings/executors/ssh-connections?id=${route.destination.id}&section=diagnostics`,
  );
  const check = page.getByRole("button", { name: "Check prerequisites", exact: true });
  await expect(
    page.getByText("This does not attempt an SSH login.", { exact: false }),
  ).toBeVisible();
  const box = await check.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(touch ? 44 : 28);
  await check.click();
  await expect(page.getByText("No setup issues found", { exact: true })).toBeVisible();
  await expect(page.getByText("SSH access verified", { exact: true })).toHaveCount(0);
  response = {
    stamp: route.stamp,
    ready: false,
    checks: [
      { code: "ssh_agent_empty", hop: "destination" },
      { code: "ssh_trust_required", hop: "bastion" },
      { code: "ssh_future_problem", hop: "" },
    ],
  };
  await check.click();
  await expect(page.getByText("3 setup issues need attention", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "View identity", exact: true })).toBeVisible();
  for (const theme of ["light", "dark"]) {
    await page.emulateMedia({ colorScheme: theme as "light" | "dark" });
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await page.screenshot({
      path: test.info().outputPath(`ssh-diagnostics-${theme}.png`),
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "View identity", exact: true }).click();
  await expect(page).toHaveURL(/tab=identities/);
  await page.getByRole("button", { name: "Back to identities", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`id=${route.destination.id}`));
  await page.getByRole("tab", { name: "Diagnostics", exact: true }).click();
  fail = true;
  await check.click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Could not complete setup checks" }),
  ).toBeVisible();
  await expect(page.getByText("3 setup issues need attention", { exact: true })).toHaveCount(0);
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await page.getByRole("alert").scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath(`ssh-diagnostics-failed-${theme}.png`) });
  }
  await page.getByRole("button", { name: "Test SSH access", exact: true }).click();
  await expect(page.getByText("SSH access could not be verified", { exact: true })).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
}
