import { expect, test, type Locator, type Page } from "@playwright/test";
import { seedSSHLayout } from "./ssh-management-layout";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
export async function exerciseSSHDiagnostics(page: Page, baseUrl: string, touch: boolean) {
  const { route } = await seedSSHLayout(baseUrl);
  route.destination.fingerprint = "SHA256:public-fixture-host";
  await page.route("**/api/v1/ssh/managed/connections", (r) => r.fulfill({ json: [route] }));
  let response = { stamp: route.stamp, ready: true, checks: [] as { code: string; hop: string }[] };
  let fail = false;
  let checkRequests = 0;
  let loginRequests = 0;
  await page.route(`**/connections/${route.destination.id}/preflight`, (r) => {
    checkRequests++;
    return r.fulfill(
      fail ? { status: 503, json: { error: "ssh_operation_failed" } } : { json: response },
    );
  });
  await page.route(`**/connections/${route.destination.id}/test`, (r) => {
    loginRequests++;
    return r.fulfill({
      status: 400,
      json: { error: "ssh_authentication_rejected", hop: "destination" },
    });
  });
  await page.goto(
    `/settings/executors/ssh-connections?id=${route.destination.id}&section=diagnostics`,
  );
  const access = page.getByRole("tab", { name: "Access", exact: true });
  await expect(access).toHaveAttribute("data-state", "active");
  await expect(page.getByRole("tab", { name: "Diagnostics", exact: true })).toHaveCount(0);
  const check = page.getByRole("button", { name: "Check setup", exact: true });
  const login = page.getByRole("button", { name: "Test SSH access", exact: true });
  const heading = page.getByRole("heading", { name: "SSH setup", exact: true });
  await expect(heading.locator("span")).toBeFocused();
  await expect(
    page.getByText("This does not attempt an SSH login.", { exact: false }),
  ).toBeVisible();
  await expect(login).toHaveCount(1);
  const liveHeading = page.getByRole("heading", { name: "SSH access", exact: true });
  const livePosition = (await liveHeading.boundingBox())!;
  const setupPosition = (await heading.boundingBox())!;
  if (touch) {
    expect(setupPosition.y).toBeGreaterThan(livePosition.y + livePosition.height);
  } else {
    expect(Math.abs(setupPosition.y - livePosition.y)).toBeLessThanOrEqual(1);
    expect(setupPosition.x).toBeGreaterThan(livePosition.x);
  }
  expect(checkRequests).toBe(0);
  expect(loginRequests).toBe(0);
  for (const control of [check, login]) {
    const height = (await control.boundingBox())!.height;
    if (touch) expect(height).toBeGreaterThanOrEqual(44);
    else expect(Math.abs(height - 28)).toBeLessThanOrEqual(1);
  }
  const activate = (control: Locator) => (touch ? control.tap() : control.press("Enter"));
  await activate(check);
  await expect(page.getByText("No setup issues found", { exact: true })).toBeVisible();
  await expect(page.getByText("SSH access verified", { exact: true })).toHaveCount(0);
  expect(checkRequests).toBe(1);
  expect(loginRequests).toBe(0);
  await activate(login);
  const loginFailure = page
    .getByRole("alert")
    .filter({ hasText: "SSH access could not be verified" });
  await expect(loginFailure).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "No setup issues found" })).toBeVisible();
  response = {
    stamp: route.stamp,
    ready: false,
    checks: [
      { code: "ssh_agent_empty", hop: "destination" },
      { code: "ssh_trust_required", hop: "bastion" },
      { code: "ssh_future_problem", hop: "" },
    ],
  };
  await activate(check);
  await expect(page.getByText("3 setup issues need attention", { exact: true })).toBeVisible();
  await expect(loginFailure).toBeVisible();
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
  await expect(access).toHaveAttribute("data-state", "active");
  await activate(check);
  await expect(page.getByText("3 setup issues need attention", { exact: true })).toBeVisible();
  fail = true;
  await activate(check);
  const checkFailure = page
    .getByRole("alert")
    .filter({ hasText: "Could not complete setup checks" });
  await expect(checkFailure).toBeVisible();
  await expect(page.getByText("3 setup issues need attention", { exact: true })).toHaveCount(0);
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await checkFailure.scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath(`ssh-diagnostics-failed-${theme}.png`) });
  }
  await activate(login);
  await expect(loginFailure).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
}
