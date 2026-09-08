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
      await testPage.route(`**/api/v1/secrets/${secret.id}/references`, async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
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
      const conflictDialog = testPage.getByTestId("secret-delete-conflict-dialog");
      await expect(conflictDialog).toBeVisible();
      await expect(testPage.getByTestId("secret-delete-confirm-popover")).toHaveCount(0);
      await expect(conflictDialog).toContainText('Agent profile "E2E review profile"');
      await expect(conflictDialog).toContainText("E2E_TOKEN");
      await expect(conflictDialog.getByTestId("secret-delete-confirm")).toHaveCount(0);
      await prCapture.screenshot("desktop-secrets-delete-conflict", {
        caption: "Desktop secret deletion conflict dialog",
      });
      await expect(row).toBeVisible();
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
      await expect(testPage.locator("body")).not.toContainText("secret_in_use");
    } finally {
      await testPage.unroute(`**/api/v1/secrets/${secret.id}/references`);
      await apiClient.deleteSecretIfPresent(secret.id);
    }
  });
});
