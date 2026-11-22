import { expect, test, type Page, type Locator } from "@playwright/test";
import { seedSSHLayout } from "./ssh-management-layout";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import pseudoSettings from "../../src/locales/pseudo/settings.json";
import { dwell } from "./causal-waits";

/** UI metadata/transport fixtures only. The containers project proves actual SSH. */
export async function exerciseSSHAccess(page: Page, baseUrl: string, touch: boolean) {
  if (!touch) await page.setViewportSize({ width: 1440, height: 1000 });
  const { identity, route } = await seedSSHLayout(baseUrl);
  route.destination.fingerprint = "SHA256:public-host-fixture";
  route.destination.verified_route = route.stamp;
  const activate = (locator: Locator) => (touch ? locator.tap() : locator.click());
  const operations: string[] = [];
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      /\/ssh\/managed\/connections\/[^/]+\/(test|trust|observe|preflight|enroll)$/.test(
        new URL(request.url()).pathname,
      )
    )
      operations.push(request.url());
  });
  await page.route("**/api/v1/ssh/managed/connections", (request) =>
    request.fulfill({ json: [route] }),
  );
  let installs = 0;
  let releaseInstall!: () => void;
  const pendingInstall = new Promise<void>((resolve) => {
    releaseInstall = resolve;
  });
  await page.route(
    `**/api/v1/ssh/managed/connections/${route.destination.id}/enroll`,
    async (request) => {
      installs++;
      const body = request.request().postDataJSON();
      expect(body.expected_stamp).toBe(route.stamp);
      expect(body.key_fingerprint).toBe(identity.fingerprint);
      if (body.mode === "password") expect(body.password === "disposable-ui-password").toBe(true);
      else {
        expect(body.mode).toBe("identity");
        expect(body.bootstrap_identity_id).toBe(identity.id);
        expect(body).not.toHaveProperty("password");
      }
      route.latest_enrollment = {
        request_id: body.request_id,
        connection_id: route.destination.id,
        stamp: route.stamp,
        key_fingerprint: identity.fingerprint,
        created_at: new Date().toISOString(),
        state: installs === 1 ? "verified" : "outcome_unknown",
      };
      if (installs === 1) await pendingInstall;
      await request.fulfill({ json: route.latest_enrollment });
    },
  );
  await page.goto(`/settings/executors/ssh-connections?id=${route.destination.id}`);
  await expect(page.getByRole("heading", { name: "Layout connection", exact: true })).toBeVisible();
  await activate(page.getByRole("tab", { name: "Access", exact: true }));
  await expect(page.getByRole("button", { name: "Install public key", exact: true })).toBeVisible();
  await expect(page.getByLabel("SSH password", { exact: true })).toHaveCount(0);
  await activate(page.getByRole("tab", { name: "Diagnostics", exact: true }));
  await expect(
    page.getByRole("button", { name: "Check prerequisites", exact: true }),
  ).toBeVisible();
  await activate(page.getByRole("tab", { name: "Access", exact: true }));
  await dwell(
    page,
    250,
    "negative-assertion",
    "opening or revisiting access and diagnostics must not invoke any SSH operation",
  );
  expect(operations).toEqual([]);
  await activate(page.getByRole("button", { name: "Install public key", exact: true }));
  await activate(page.getByRole("radio", { name: "Install manually", exact: true }));
  await expect(page.getByRole("button", { name: "Copy public key", exact: true })).toBeVisible();
  expect(installs).toBe(0);
  await activate(page.getByRole("radio", { name: "Password", exact: true }));
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await page
      .getByRole("heading", { name: "Install public key", exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath(`ssh-install-choice-${theme}.png`) });
  }
  const password = page.getByLabel("SSH password", { exact: true });
  await password.fill("discarded-ui-password");
  await activate(page.getByRole("tab", { name: "Overview", exact: true }));
  await expect(password).toHaveCount(0);
  await activate(page.getByRole("tab", { name: "Access", exact: true }));
  await expect(password).toHaveCount(0);
  await activate(page.getByRole("button", { name: "Install public key", exact: true }));
  await activate(page.getByRole("radio", { name: "Password", exact: true }));
  await expect(password).toHaveValue("");
  await password.fill("disposable-ui-password");
  await activate(page.getByRole("button", { name: "Review installation", exact: true }));
  const confirmation = page.getByRole("region", { name: "Authorize this key", exact: true });
  await expect(confirmation).toContainText("deploy@layout.invalid:22");
  await expect(confirmation).toContainText(route.destination.fingerprint);
  await expect(confirmation).toContainText(identity.fingerprint);
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await confirmation
      .getByRole("button", { name: "Install public key", exact: true })
      .scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath(`ssh-install-review-${theme}.png`) });
  }
  const installed = page.waitForResponse((response) =>
    response.url().endsWith(`/${route.destination.id}/enroll`),
  );
  await activate(confirmation.getByRole("button", { name: "Install public key", exact: true }));
  try {
    await expect(password).toHaveCount(0);
    const running = page
      .getByRole("status")
      .filter({ hasText: "Installing the key and checking access" });
    await expect(running).toBeVisible();
    for (const theme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme: theme });
      await running.scrollIntoViewIfNeeded();
      await page.screenshot({ path: test.info().outputPath(`ssh-install-pending-${theme}.png`) });
    }
  } finally {
    releaseInstall();
  }
  expect((await installed).ok()).toBe(true);
  await expect(password).toHaveCount(0);
  await expect(
    page.getByText("Key installed and remote access verified.", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Test SSH access", exact: true })).toBeFocused();
  expect(installs).toBe(1);
  await assertNoDocumentHorizontalOverflow(page);
  await page.screenshot({
    path: test.info().outputPath("ssh-access-verified.png"),
    fullPage: true,
  });
  await activate(page.getByRole("button", { name: "Install public key", exact: true }));
  await activate(page.getByRole("radio", { name: "Password", exact: true }));
  await expect(password).toHaveValue("");
  await activate(page.getByRole("button", { name: "Close setup", exact: true }));
  await expect(password).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Install public key", exact: true })).toBeFocused();
  await activate(page.getByRole("button", { name: "Install public key", exact: true }));
  await activate(page.getByRole("radio", { name: "Existing identity", exact: true }));
  const picker = page.locator("#ssh-enrollment-identity");
  if (await picker.evaluate((element) => element.tagName === "SELECT"))
    await picker.selectOption(identity.id);
  else {
    await activate(picker);
    await activate(page.getByRole("option", { name: identity.label, exact: true }));
  }
  await activate(page.getByRole("button", { name: "Review installation", exact: true }));
  await expect(confirmation).toContainText(identity.label);
  await activate(confirmation.getByRole("button", { name: "Install public key", exact: true }));
  const uncertain = page.getByRole("status").filter({ hasText: "Enrollment was interrupted" });
  await expect(uncertain).toBeVisible();
  await expect(password).toHaveCount(0);
  for (const theme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: theme });
    await uncertain.scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath(`ssh-install-unknown-${theme}.png`) });
  }
  expect(installs).toBe(2);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo");
  await page
    .getByRole("button", { name: pseudoSettings["sshManager.ui.installKey"], exact: true })
    .click();
  await expect(
    page.getByRole("radio", {
      name: pseudoSettings["sshManager.experience.methodIdentity"],
      exact: true,
    }),
  ).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
  await page.screenshot({ path: test.info().outputPath("ssh-install-pseudo.png") });
}
