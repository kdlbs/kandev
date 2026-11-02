import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

/** Unique per run so retries never collide with leftovers from a failed attempt. */
const runToken = () => `${Date.now()}${randomUUID().slice(0, 8)}`;

test.describe("workspace secrets edit", () => {
  test("edits an existing workspace secret value and name", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const name = `E2E Edit Secret ${runToken()}`;
    const renamed = `${name} Renamed`;
    const originalValue = "e2e-edit-original-value";
    const newValue = "e2e-edit-new-value";
    await apiClient.createSecret(name, originalValue, {
      scope: "workspace",
      workspaceId: seedData.workspaceId,
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/secrets`);
    await expect(testPage.getByText(name, { exact: true })).toBeVisible();

    await testPage.getByRole("button", { name: `Edit secret ${name}` }).click();
    await testPage.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(renamed);
    await testPage.getByPlaceholder("Secret value").fill(newValue);

    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    const updateResponse = waitForHttp(testPage, "PUT", /\/api\/v1\/secrets\/[^/]+$/, {
      predicate: (response) => response.status() === 200,
    });
    await save.click();
    await updateResponse;

    await expect(testPage.getByText(renamed, { exact: true })).toBeVisible();
    await expect(testPage.getByText(name, { exact: true })).toBeHidden();

    await testPage.reload();
    await expect(testPage.getByText(renamed, { exact: true })).toBeVisible();
    const revealed = waitForHttp(testPage, "POST", /\/api\/v1\/secrets\/[^/]+\/reveal$/);
    await testPage.getByRole("button", { name: `Reveal secret ${renamed}` }).click();
    expect((await revealed).status()).toBe(200);
    await expect(testPage.getByText(newValue)).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(originalValue);
  });

  test("hides a revealed value when an edit saves", async ({ testPage, apiClient, seedData }) => {
    const name = `E2E Hide Reveal ${runToken()}`;
    const originalValue = "e2e-hide-original-value";
    const newValue = "e2e-hide-new-value";
    await apiClient.createSecret(name, originalValue, {
      scope: "workspace",
      workspaceId: seedData.workspaceId,
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/secrets`);
    const originalReveal = waitForHttp(testPage, "POST", /\/api\/v1\/secrets\/[^/]+\/reveal$/);
    await testPage.getByRole("button", { name: `Reveal secret ${name}` }).click();
    expect((await originalReveal).status()).toBe(200);
    await expect(testPage.getByText(originalValue)).toBeVisible();

    await testPage.getByRole("button", { name: `Edit secret ${name}` }).click();
    await expect(testPage.getByRole("button", { name: `Reveal secret ${name}` })).toBeHidden();
    await testPage.getByPlaceholder("Secret value").fill(newValue);
    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    const updateResponse = waitForHttp(testPage, "PUT", /\/api\/v1\/secrets\/[^/]+$/, {
      predicate: (response) => response.status() === 200,
    });
    await save.click();
    await updateResponse;

    await expect(testPage.locator("body")).not.toContainText(originalValue);
    const rotatedReveal = waitForHttp(testPage, "POST", /\/api\/v1\/secrets\/[^/]+\/reveal$/);
    await testPage.getByRole("button", { name: `Reveal secret ${name}` }).click();
    expect((await rotatedReveal).status()).toBe(200);
    await expect(testPage.getByText(newValue)).toBeVisible();
  });
});
