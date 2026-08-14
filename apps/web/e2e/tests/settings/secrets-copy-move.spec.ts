import { randomUUID } from "node:crypto";

import { test, expect } from "../../fixtures/test-base";

const GLOBAL_VALUE = "e2e-copy-move-global-value";

/** Unique per run so retries never collide with leftovers from a failed attempt. */
const runToken = () => `${Date.now()}${randomUUID().slice(0, 8)}`;
const WORKSPACE_VALUE = "e2e-copy-move-workspace-value";

/** Creates a secret from the settings page via the Add secret dialog. */
async function createSecretFromSettings(
  page: import("@playwright/test").Page,
  route: string,
  name: string,
  value: string,
) {
  await page.goto(route);
  await expect(page.getByRole("button", { name: "Add secret", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add secret", exact: true }).click();
  await page.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(name);
  await page.getByPlaceholder("Secret value").fill(value);
  const save = page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  await expect(save).toBeEnabled();
  await save.click();
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(value);
}

/**
 * Opens the copy/move dialog for a named secret and submits through the
 * dialog's OWN primary button (never the settings floating Save control).
 */
async function submitCopyMove(
  page: import("@playwright/test").Page,
  secretName: string,
  mode: "Copy" | "Move",
) {
  await page.getByRole("button", { name: `Copy or move ${secretName}` }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  if (mode === "Move") {
    await dialog.getByRole("radio", { name: /Move/ }).click();
  }
  await dialog.getByRole("button", { name: mode, exact: true }).click();
  await expect(dialog).toBeHidden();
}

test.describe("secrets-copy-move", () => {
  test("copies a global secret to a workspace with the default suffixed name", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sourceName = `E2E Copy Move Global Copy ${runToken()}`;
    const copiedName = `${sourceName} (from Global)`;
    await createSecretFromSettings(testPage, "/settings/general/secrets", sourceName, GLOBAL_VALUE);
    const global = (await apiClient.listSecrets()).find((secret) => secret.name === sourceName);
    expect(global?.id).toBeTruthy();

    await submitCopyMove(testPage, sourceName, "Copy");

    // The source stays on the Global page; the copy appears in the workspace.
    await expect(testPage.getByText(sourceName, { exact: true })).toBeVisible();
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await expect(testPage.getByText(copiedName, { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(GLOBAL_VALUE);

    const copied = (
      await apiClient.listSecrets({ scope: "workspace", workspaceId: seedData.workspaceId })
    ).find((secret) => secret.name === copiedName);
    expect(copied?.scope).toBe("workspace");
    expect(copied?.workspace_id).toBe(seedData.workspaceId);
  });

  test("moves a workspace secret to General and removes the source", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sourceName = `E2E Copy Move Workspace Move ${runToken()}`;
    await createSecretFromSettings(
      testPage,
      `/settings/workspace/${seedData.workspaceId}/secrets`,
      sourceName,
      WORKSPACE_VALUE,
    );
    const workspaces = await apiClient.listWorkspaces();
    const workspaceName = workspaces.workspaces.find(
      (workspace) => workspace.id === seedData.workspaceId,
    )?.name;
    expect(workspaceName).toBeTruthy();
    const movedName = `${sourceName} (from ${workspaceName})`;

    await submitCopyMove(testPage, sourceName, "Move");

    await expect(testPage.getByText(sourceName, { exact: true })).toHaveCount(0);
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);

    await testPage.goto("/settings/general/secrets");
    await expect(testPage.getByText(movedName, { exact: true })).toBeVisible();
    const moved = (await apiClient.listSecrets()).find((secret) => secret.name === movedName);
    expect(moved?.scope).toBe("global");
    const workspaceSecret = (
      await apiClient.listSecrets({ scope: "workspace", workspaceId: seedData.workspaceId })
    ).find((secret) => secret.name === sourceName);
    expect(workspaceSecret).toBeUndefined();
  });

  test("moves a global secret to a workspace and removes the source", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sourceName = `E2E Copy Move Global Move ${runToken()}`;
    const movedName = `${sourceName} (from Global)`;
    await createSecretFromSettings(testPage, "/settings/general/secrets", sourceName, GLOBAL_VALUE);

    await submitCopyMove(testPage, sourceName, "Move");

    await expect(testPage.getByText(sourceName, { exact: true })).toHaveCount(0);
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
    await expect(testPage.getByText(movedName, { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(GLOBAL_VALUE);

    const global = (await apiClient.listSecrets()).find((secret) => secret.name === sourceName);
    expect(global).toBeUndefined();
    const workspace = (
      await apiClient.listSecrets({ scope: "workspace", workspaceId: seedData.workspaceId })
    ).find((secret) => secret.name === movedName);
    expect(workspace?.workspace_id).toBe(seedData.workspaceId);
  });

  test("blocks a duplicate target name and marks the name field invalid", async ({
    testPage,
    seedData,
  }) => {
    const sourceName = `E2E Copy Move Conflict Source ${runToken()}`;
    const conflictingName = `${sourceName} (from Global)`;
    await createSecretFromSettings(testPage, "/settings/general/secrets", sourceName, GLOBAL_VALUE);
    await createSecretFromSettings(
      testPage,
      `/settings/workspace/${seedData.workspaceId}/secrets`,
      conflictingName,
      WORKSPACE_VALUE,
    );

    await testPage.goto("/settings/general/secrets");
    await testPage.getByRole("button", { name: `Copy or move ${sourceName}` }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const nameInput = dialog.getByLabel("Name");
    await expect(nameInput).toHaveValue(conflictingName);
    await expect(nameInput).toHaveAttribute("aria-invalid", "true");
    await expect(
      dialog.getByText(`A secret named ${conflictingName} already exists in this destination.`),
    ).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Copy", exact: true })).toBeDisabled();

    // Editing the name clears the conflict and re-enables submission.
    await nameInput.fill(`${sourceName} (copy)`);
    await expect(nameInput).not.toHaveAttribute("aria-invalid", "true");
    await expect(dialog.getByRole("button", { name: "Copy", exact: true })).toBeEnabled();
  });

  test("restores focus to the trigger after closing the dialog", async ({ testPage }) => {
    // Unique per run: retries must not collide with a secret a previous
    // attempt left behind (the fixture reset does not remove secrets).
    const sourceName = `E2E Copy Move Focus ${runToken()}`;
    await createSecretFromSettings(testPage, "/settings/general/secrets", sourceName, GLOBAL_VALUE);

    await testPage.getByRole("button", { name: `Copy or move ${sourceName}` }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(dialog).toBeHidden();
    // Desktop mouse clicks focus the trigger; closing the dialog restores it.
    await expect
      .poll(async () =>
        testPage.evaluate(() => document.activeElement?.getAttribute("aria-label") ?? ""),
      )
      .toContain(`Copy or move ${sourceName}`);
  });
});
