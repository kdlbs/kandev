import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";

const SECRET_VALUE = "e2e-secret-delete-redaction-value";

function runToken() {
  return `${Date.now()}-${randomUUID().slice(0, 8)}`;
}

test.describe("Secret deletion", () => {
  test("confirms at the target row, supports cancel, and removes the secret", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const name = `E2E Delete Secret ${runToken()}`;
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
      await expect(trigger).toBeVisible();
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);

      await trigger.click();
      const confirmation = testPage.getByTestId("secret-delete-confirm-popover");
      await expect(confirmation).toBeVisible();
      await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
      await expect(confirmation).not.toContainText(SECRET_VALUE);
      await prCapture.screenshot("desktop-secrets-delete-confirmation", {
        caption: "Desktop secret row confirmation",
      });

      await confirmation.getByRole("button", { name: "Cancel" }).click();
      await expect(confirmation).toBeHidden();
      await expect(trigger).toBeFocused();
      expect((await apiClient.listSecrets()).some((item) => item.id === secret.id)).toBe(true);

      await trigger.click();
      await testPage.getByTestId("secret-delete-confirm").click();
      await expect(row).toHaveCount(0);
      await expect
        .poll(async () => (await apiClient.listSecrets()).some((item) => item.id === secret.id))
        .toBe(false);
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
    } finally {
      await apiClient.deleteSecretIfPresent(secret.id);
    }
  });

  test("keeps the target row and explains an in-use conflict", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const name = `E2E Failed Delete Secret ${runToken()}`;
    const secret = await apiClient.createSecret(name, SECRET_VALUE);

    try {
      await expect
        .poll(async () => (await apiClient.listSecrets()).some((item) => item.id === secret.id), {
          timeout: 30_000,
        })
        .toBe(true);
      await testPage.goto("/settings/general/secrets");
      const row = testPage.getByTestId(`secret-row-${secret.id}`);
      await testPage.route(`**/api/v1/secrets/${secret.id}`, async (route) => {
        await route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({
            code: "secret_in_use",
            references: [
              {
                kind: "agent_profile",
                name: "E2E review profile",
                key: "E2E_TOKEN",
              },
            ],
          }),
        });
      });
      await row.getByRole("button", { name: `Delete secret ${name}` }).click();
      const confirmation = testPage.getByTestId("secret-delete-confirm-popover");
      await testPage.getByTestId("secret-delete-confirm").click();
      await expect(confirmation).toBeHidden();

      const toast = testPage.getByTestId("toast-message");
      await expect(toast).toContainText(
        'This secret is in use by: Agent profile "E2E review profile" (E2E_TOKEN). Remove or replace these references before deleting it.',
      );
      await expect
        .poll(async () => {
          const box = await toast.boundingBox();
          const viewport = testPage.viewportSize();
          return Boolean(box && viewport && box.x >= 0 && box.x + box.width <= viewport.width);
        })
        .toBe(true);
      await prCapture.screenshot("desktop-secrets-delete-conflict", {
        caption: "Desktop secret deletion conflict toast",
      });
      await expect(row).toBeVisible();
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
      await expect(testPage.locator("body")).not.toContainText("secret_in_use");
    } finally {
      await testPage.unroute(`**/api/v1/secrets/${secret.id}`);
      await apiClient.deleteSecretIfPresent(secret.id);
    }
  });
});
