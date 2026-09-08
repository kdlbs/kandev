import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";

const SECRET_VALUE = "e2e-secret-delete-redaction-value";
const REFERENCE_KINDS = ["agent_profile", "executor_profile", "repository"] as const;

function runToken() {
  return `${Date.now()}-${randomUUID().slice(0, 8)}`;
}

function conflictReferences() {
  return Array.from({ length: 18 }, (_, index) => ({
    kind: REFERENCE_KINDS[index % REFERENCE_KINDS.length],
    name: index === 0 ? "E2E review profile" : `E2E resource ${index + 1}`,
    key: index === 0 ? "E2E_TOKEN" : `E2E_TOKEN_${index + 1}`,
  }));
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
            references: conflictReferences(),
          }),
        });
      });
      await row.getByRole("button", { name: `Delete secret ${name}` }).click();
      const conflictDialog = testPage.getByTestId("secret-delete-conflict-dialog");
      await expect(conflictDialog).toBeVisible();
      await expect(testPage.getByTestId("secret-delete-confirm-popover")).toHaveCount(0);
      const referenceList = conflictDialog.getByTestId("secret-delete-reference-list");
      const referenceCards = referenceList.getByTestId("secret-delete-reference");
      await expect(referenceCards).toHaveCount(18);
      await expect(referenceCards.first()).toContainText("E2E review profile");
      await expect(referenceCards.first()).toContainText("Agent profile");
      await expect(referenceCards.first()).toContainText("E2E_TOKEN");
      await expect(conflictDialog.getByTestId("secret-delete-confirm")).toHaveCount(0);
      const listDimensions = await referenceList.evaluate((element) => ({
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      }));
      expect(listDimensions.scrollHeight).toBeGreaterThan(listDimensions.clientHeight);
      await prCapture.screenshot("desktop-secrets-delete-conflict", {
        caption: "Desktop secret deletion conflict dialog",
      });
      await referenceList.evaluate((element) => element.scrollTo({ top: element.scrollHeight }));
      await expect(referenceCards.last()).toBeInViewport();
      await expect(conflictDialog.getByRole("button", { name: "Close" })).toBeVisible();
      await expect(row).toBeVisible();
      await expect(testPage.locator("body")).not.toContainText(SECRET_VALUE);
      await expect(testPage.locator("body")).not.toContainText("secret_in_use");
    } finally {
      await testPage.unroute(`**/api/v1/secrets/${secret.id}/references`);
      await apiClient.deleteSecretIfPresent(secret.id);
    }
  });
});
