import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const SECRET_VALUE = "e2e-mobile-secret-delete-redaction-value";

function runToken() {
  return `${Date.now()}-${randomUUID().slice(0, 8)}`;
}

test.describe("mobile-secrets-delete", () => {
  test("confirms inline at the target row without exposing the value", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const name = `E2E Mobile Delete Secret ${runToken()}`;
    const secret = await apiClient.createSecret(name, SECRET_VALUE);

    try {
      await expect
        .poll(async () => (await apiClient.listSecrets()).some((item) => item.id === secret.id), {
          timeout: 30_000,
        })
        .toBe(true);
      await testPage.goto("/settings/general/secrets");
      const row = testPage.getByTestId(`secret-row-${secret.id}`);
      const trigger = row.getByRole("button", { name: `Delete secret ${name}` });
      await trigger.tap();

      const inline = row.getByTestId("secret-delete-inline-confirmation");
      await expect(inline).toBeVisible();
      await expect(inline).toContainText(
        `This will permanently remove ${name}. This action cannot be undone.`,
      );
      const warningBox = await inline.locator("p").boundingBox();
      expect(warningBox).not.toBeNull();
      expect(warningBox!.x).toBeGreaterThanOrEqual(0);
      expect(warningBox!.x + warningBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      await expect(testPage.getByTestId("secret-delete-confirm-popover")).toHaveCount(0);
      await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
      await expect(inline).not.toContainText(SECRET_VALUE);
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);

      for (const control of [
        inline.getByRole("button", { name: "Cancel" }),
        inline.getByTestId("secret-delete-confirm"),
      ]) {
        const box = await control.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      await assertNoDocumentHorizontalOverflow(testPage, "mobile secret deletion confirmation");
      await prCapture.screenshot("mobile-secrets-delete-confirmation", {
        caption: "Mobile secret row inline confirmation",
      });

      await inline.getByRole("button", { name: "Cancel" }).tap();
      await expect(inline).toHaveCount(0);
      expect((await apiClient.listSecrets()).some((item) => item.id === secret.id)).toBe(true);

      await row.getByRole("button", { name: `Delete secret ${name}` }).tap();
      await row
        .getByTestId("secret-delete-inline-confirmation")
        .getByTestId("secret-delete-confirm")
        .tap();
      await expect(row).toHaveCount(0);
      await expect
        .poll(async () => (await apiClient.listSecrets()).some((item) => item.id === secret.id))
        .toBe(false);
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
    } finally {
      await apiClient.deleteSecretIfPresent(secret.id).catch(() => undefined);
    }
  });

  test("keeps the target row and explains an in-use conflict", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const name = `E2E Mobile Failed Delete Secret ${runToken()}`;
    const secret = await apiClient.createSecret(name, SECRET_VALUE);

    try {
      await expect
        .poll(async () => (await apiClient.listSecrets()).some((item) => item.id === secret.id), {
          timeout: 30_000,
        })
        .toBe(true);
      await testPage.goto("/settings/general/secrets");
      const row = testPage.getByTestId(`secret-row-${secret.id}`);
      await testPage.route(`**/api/v1/secrets/${secret.id}/references`, async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            references: [
              {
                kind: "agent_profile",
                name: "E2E mobile review profile",
                key: "E2E_MOBILE_TOKEN",
              },
            ],
          }),
        });
      });

      await row.getByRole("button", { name: `Delete secret ${name}` }).tap();
      const conflictDialog = testPage.getByTestId("secret-delete-conflict-dialog");
      await expect(conflictDialog).toBeVisible();
      await expect(conflictDialog).toContainText('Agent profile "E2E mobile review profile"');
      await expect(conflictDialog).toContainText("E2E_MOBILE_TOKEN");
      await expect(conflictDialog.getByTestId("secret-delete-confirm")).toHaveCount(0);
      const dialogBox = await conflictDialog.boundingBox();
      expect(dialogBox).not.toBeNull();
      expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
      expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile secret deletion conflict");
      await prCapture.screenshot("mobile-secrets-delete-conflict", {
        caption: "Mobile secret deletion conflict dialog",
      });
      await expect(row).toBeVisible();
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
      await expect(testPage.locator("body")).not.toContainText("secret_in_use");
    } finally {
      await testPage.unroute(`**/api/v1/secrets/${secret.id}/references`);
      await apiClient.deleteSecretIfPresent(secret.id).catch(() => undefined);
    }
  });
});
