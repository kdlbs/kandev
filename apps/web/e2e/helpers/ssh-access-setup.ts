import { expect, test, type Page, type Locator } from "@playwright/test";
import { seedSSHLayout } from "./ssh-management-layout";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { dwell } from "./causal-waits";

/** UI metadata/transport fixtures only. The containers project proves actual SSH. */
export async function exerciseSSHAccess(page: Page, baseUrl: string, touch: boolean) {
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
  await page.route(
    `**/api/v1/ssh/managed/connections/${route.destination.id}/enroll`,
    async (request) => {
      installs++;
      const body = request.request().postDataJSON();
      expect(body.expected_stamp).toBe(route.stamp);
      expect(body.key_fingerprint).toBe(identity.fingerprint);
      expect(body.password === "disposable-ui-password").toBe(true);
      route.latest_enrollment = {
        request_id: body.request_id,
        connection_id: route.destination.id,
        stamp: route.stamp,
        key_fingerprint: identity.fingerprint,
        created_at: new Date().toISOString(),
        state: "verified",
      };
      await request.fulfill({ json: route.latest_enrollment });
    },
  );
  await page.goto(`/settings/executors/ssh-connections?id=${route.destination.id}`);
  await expect(page.getByRole("heading", { name: "Layout connection", exact: true })).toBeVisible();
  await activate(page.getByRole("tab", { name: "Access", exact: true }));
  await expect(page.getByRole("button", { name: "Install key", exact: true })).toBeVisible();
  await expect(page.getByLabel("One-time password", { exact: true })).toHaveCount(0);
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
  await activate(page.getByRole("button", { name: "Install key", exact: true }));
  const password = page.getByLabel("One-time password", { exact: true });
  await password.fill("discarded-ui-password");
  await activate(page.getByRole("tab", { name: "Overview", exact: true }));
  await expect(password).toHaveCount(0);
  await activate(page.getByRole("tab", { name: "Access", exact: true }));
  await expect(password).toHaveCount(0);
  await activate(page.getByRole("button", { name: "Install key", exact: true }));
  await expect(password).toHaveValue("");
  await password.fill("disposable-ui-password");
  await activate(page.getByRole("button", { name: "Enroll key", exact: true }));
  const confirmation = page.getByRole("dialog", {
    name: "Authorize this key on the remote account?",
  });
  await expect(confirmation).toContainText("deploy@layout.invalid:22");
  await expect(confirmation).toContainText(route.destination.fingerprint);
  await expect(confirmation).toContainText(identity.fingerprint);
  const installed = page.waitForResponse((response) =>
    response.url().endsWith(`/${route.destination.id}/enroll`),
  );
  await activate(confirmation.getByRole("button", { name: "Enroll key", exact: true }));
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
  await activate(page.getByRole("button", { name: "Install key", exact: true }));
  await expect(password).toHaveValue("");
  await activate(page.getByRole("button", { name: "Close setup", exact: true }));
  await expect(password).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Install key", exact: true })).toBeFocused();
}
