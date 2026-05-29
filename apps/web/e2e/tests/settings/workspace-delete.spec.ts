import { test, expect } from "../../fixtures/test-base";

test.describe("Workspace settings deletion", () => {
  test("deletes a workspace from the settings edit page", async ({ testPage, apiClient }) => {
    const suffix = Date.now().toString(36);
    const workspaceName = `Settings Delete ${suffix}`;
    const workspace = await apiClient.createWorkspace(workspaceName);

    await testPage.goto(`/settings/workspace/${workspace.id}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();

    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    // The wrong confirmation string ("delete") must not enable deletion — the
    // backend requires confirm_name to equal the workspace name.
    await confirmInput.fill("delete");
    await expect(confirmButton).toBeDisabled();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Deletion runs through a Next.js server action (the DELETE to the backend
    // happens server-side), so assert the user-visible outcome: redirect to the
    // workspace list and the workspace gone from the backend.
    await confirmButton.click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === workspace.id)).toBe(false);
  });
});
