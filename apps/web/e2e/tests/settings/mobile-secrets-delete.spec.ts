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
});
