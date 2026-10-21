import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { settledBoundingBox } from "../../helpers/settled-box";

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.15
for (const scope of ["workspace", "global"] as const) {
  test(`edits and re-reveals a ${scope} secret on a phone after reload`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const name = `Phone edit ${randomUUID().slice(0, 8)}`;
    const renamed = `${name} rotated`;
    const originalValue = "phone-edit-original";
    const rotatedValue = "phone-edit-rotated";
    const secret = await apiClient.createSecret(name, originalValue, {
      scope,
      ...(scope === "workspace" ? { workspaceId: seedData.workspaceId } : {}),
    });
    const path =
      scope === "workspace"
        ? `/settings/workspaces/${seedData.workspaceId}/secrets`
        : "/settings/general/secrets";
    await testPage.goto(path);
    const row = testPage.getByTestId(`secret-row-${secret.id}`);
    await expect(row).toHaveCount(1);
    const revealed = waitForHttp(testPage, "POST", /\/api\/v1\/secrets\/[^/?]+\/reveal/);
    await row.getByRole("button", { name: `Reveal secret ${name}`, exact: true }).tap();
    await revealed;
    await expect(row.getByText(originalValue, { exact: true })).toBeVisible();
    const edit = row.getByRole("button", { name: `Edit secret ${name}`, exact: true });
    const editBox = await settledBoundingBox(edit);
    expect(editBox.height).toBeGreaterThanOrEqual(44);
    expect(editBox.width).toBeGreaterThanOrEqual(44);
    await edit.tap();
    await expect(row).toHaveCount(0);
    await expect(testPage.locator("body")).not.toContainText(originalValue);
    const nameInput = testPage.getByPlaceholder("Name (e.g. OpenAI Production Key)");
    await nameInput.fill(renamed);
    await testPage.getByPlaceholder("Secret value").fill(rotatedValue);
    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true });
    await expect(save).toBeEnabled();
    const saveBox = await settledBoundingBox(save);
    expect(saveBox.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "phone secret edit form");
    const saved = waitForHttp(testPage, "PUT", /\/api\/v1\/secrets\/[^/]+$/, {
      predicate: (response) => response.status() === 200,
    });
    await save.tap();
    await saved;
    await expect(row.getByText(renamed, { exact: true })).toBeVisible();
    await expect(testPage.getByPlaceholder("Secret value")).toHaveCount(0);
    await expect(testPage.locator("body")).not.toContainText(originalValue);
    await expect(testPage.locator("body")).not.toContainText(rotatedValue);

    await testPage.reload();
    await expect(row.getByText(renamed, { exact: true })).toBeVisible();
    const rerevealed = waitForHttp(testPage, "POST", /\/api\/v1\/secrets\/[^/?]+\/reveal/);
    await row.getByRole("button", { name: `Reveal secret ${renamed}`, exact: true }).tap();
    await rerevealed;
    await expect(row.getByText(rotatedValue, { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(originalValue);
    await assertNoDocumentHorizontalOverflow(testPage, "phone secret edit result");
  });
}
