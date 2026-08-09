import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";

// Regression guard for workspace deletion on a non-office install. Settings
// deletion used to always call `/api/v1/office/workspaces/:id`, but the whole
// `/api/v1/office` router group is only mounted when the office services exist,
// and those are skipped when `features.office` is off — the production default.
// The rest of the suite runs with office ON (see profiles.yaml), which is why
// `workspace-delete.spec.ts` never caught this. Keep this file separate: the
// office-off backend restart is file-scoped.
useRegularMode();

test.describe("Workspace settings without the office feature", () => {
  test("deletes a workspace from the settings edit page", async ({ testPage, apiClient }) => {
    const suffix = Date.now().toString(36);
    const workspaceName = `Regular Delete ${suffix}`;
    const workspace = await apiClient.createWorkspace(workspaceName);

    await testPage.goto(`/settings/workspace/${workspace.id}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();

    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Assert the request target, not just the outcome: hitting the office route
    // here is the exact bug, and a 404 would otherwise surface only as a toast.
    const deleteRequest = testPage.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/workspaces/${workspace.id}`) &&
        response.request().method() === "DELETE",
      // Below the 60s spec timeout so a regression reports this wait as the
      // failure rather than timing out the whole test.
      { timeout: 15_000 },
    );
    await confirmButton.click();
    expect((await deleteRequest).ok()).toBe(true);

    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === workspace.id)).toBe(false);
  });
});
