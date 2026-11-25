import { expect, type Page } from "@playwright/test";
import type { SSHIdentity, SSHRoute } from "../../lib/types/http-ssh-connections";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

// The UI contract uses a simulated API response. Real SSH enrollment has separate wire tests.
export async function exerciseSSHEnrollmentConfirmation(page: Page, identity: SSHIdentity) {
  const saved: SSHRoute = {
    destination: {
      id: "confirmation-fixture",
      name: "Fixture target",
      host: "target.invalid",
      port: 22,
      user: "deploy",
      identity_id: identity.id,
      fingerprint: "SHA256:fixture-host",
      public_key: "ssh-ed25519 fixture-host",
      version: 1,
      revision: 1,
    },
    stamp: "fixture-route",
  };
  let submissions = 0;
  await page.route("**/api/v1/ssh/managed/connections", (request) =>
    request.fulfill({ json: [saved] }),
  );
  await page.route(
    "**/api/v1/ssh/managed/connections/confirmation-fixture/enroll",
    async (request) => {
      submissions++;
      const body = request.request().postDataJSON() as {
        password: string;
        expected_stamp: string;
        key_fingerprint: string;
        request_id: string;
      };
      expect(body.password === "disposable-browser-password").toBe(true);
      expect(body.expected_stamp).toBe(saved.stamp);
      expect(body.key_fingerprint).toBe(identity.fingerprint);
      saved.latest_enrollment = {
        request_id: body.request_id,
        connection_id: saved.destination.id,
        stamp: saved.stamp,
        key_fingerprint: identity.fingerprint,
        created_at: new Date().toISOString(),
        state: "installed_unverified",
        error_code: "ssh_authentication_rejected",
        hop: "destination",
      };
      await request.fulfill({ json: saved.latest_enrollment });
    },
  );
  await page.goto(
    "/settings/ssh-connections?tab=connections&id=confirmation-fixture&section=access",
  );
  await page.getByRole("button", { name: "Install public key", exact: true }).click();
  await page.getByRole("radio", { name: "Password", exact: true }).click();
  const password = page.getByLabel("SSH password", { exact: true });
  const install = page.getByRole("button", { name: "Review installation", exact: true });
  await password.fill("disposable-browser-password");
  await password.press("Enter");
  const confirmation = page.getByRole("region", { name: "Authorize this key", exact: true });
  await expect(confirmation).toContainText("deploy@target.invalid:22");
  await expect(confirmation).toContainText("SHA256:fixture-host");
  await expect(confirmation).toContainText(identity.fingerprint);
  await confirmation.getByRole("button", { name: "Back", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Review installation" })).toHaveCount(0);
  await expect(password).toHaveValue("");
  await expect(password).toBeFocused();
  expect(submissions).toBe(0);
  await password.fill("disposable-browser-password");
  await install.click();
  await confirmation.getByRole("button", { name: "Install public key", exact: true }).click();
  await expect(password).toHaveCount(0);
  await expect(
    page.getByText(
      "Destination: The remote account rejected this identity. Authorize its public key or choose another identity.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "The key may be installed" }),
  ).toBeVisible();
  expect(submissions).toBe(1);
  await expect(confirmation).toBeVisible();
  await page.getByRole("button", { name: "Close setup", exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Install public key", exact: true })).toBeFocused();
  await assertNoDocumentHorizontalOverflow(page);
}
