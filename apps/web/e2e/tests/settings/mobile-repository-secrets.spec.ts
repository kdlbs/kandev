import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const WORKSPACE_VALUE = "e2e-mobile-workspace-secret-value";

test.describe("Mobile repository secrets", () => {
  test("manages a workspace secret and repository binding without overflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}`);

    await testPage.getByTestId("settings-mobile-menu-button").click();
    const menu = testPage.getByTestId("settings-mobile-menu");
    await expect(menu).toBeVisible();
    await menu.locator(`a[href="/settings/workspace/${seedData.workspaceId}/secrets"]`).click();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspace/${seedData.workspaceId}/secrets$`),
    );

    await testPage.getByRole("button", { name: "Add secret", exact: true }).click();
    await testPage
      .getByPlaceholder("Name (e.g. OpenAI Production Key)")
      .fill("E2E Mobile Workspace Secret");
    await testPage.getByPlaceholder("Secret value").fill(WORKSPACE_VALUE);
    const secretSave = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(secretSave).toBeEnabled();
    await secretSave.click();
    await expect(testPage.getByText("E2E Mobile Workspace Secret", { exact: true })).toBeVisible();
    await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);

    const workspaceSecret = (
      await apiClient.listSecrets({
        scope: "workspace",
        workspaceId: seedData.workspaceId,
      })
    ).find((secret) => secret.name === "E2E Mobile Workspace Secret");
    expect(workspaceSecret?.id).toBeTruthy();
    if (!workspaceSecret) throw new Error("mobile workspace secret was not persisted");

    try {
      await testPage.getByTestId("settings-mobile-menu-button").click();
      await testPage
        .getByTestId("settings-mobile-menu")
        .getByRole("link", { name: "Repositories", exact: true })
        .click();
      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspace/${seedData.workspaceId}/repositories$`),
      );
      await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();

      const editor = testPage.getByTestId("repository-secret-bindings");
      await editor.getByTestId("repository-secret-add").click();
      await editor.getByTestId("repository-secret-key-0").fill("E2E_MOBILE_TOKEN");
      await editor.getByTestId("repository-secret-select-0").click();
      await testPage
        .getByRole("option", {
          name: "E2E Mobile Workspace Secret - Workspace",
          exact: true,
        })
        .click();

      const repositorySaveResponse = testPage.waitForResponse(
        (response) =>
          response.url().endsWith(`/api/v1/repositories/${seedData.repositoryId}`) &&
          response.request().method() === "PATCH" &&
          response.ok(),
      );
      const repositorySave = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });
      await expect(repositorySave).toBeEnabled();
      await repositorySave.click();
      await repositorySaveResponse;
      await expect(repositorySave).toHaveCount(0);

      for (const control of [
        editor.getByTestId("repository-secret-key-0"),
        editor.getByTestId("repository-secret-select-0"),
        editor.getByTestId("repository-secret-add"),
      ]) {
        const box = await control.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      await assertNoDocumentHorizontalOverflow(testPage, "mobile repository secrets");
      await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);
      await prCapture.screenshot("mobile-repository-secret-bindings", {
        caption: "Mobile repository settings with a workspace secret binding",
      });

      await testPage.reload();
      await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();
      const reloaded = testPage.getByTestId("repository-secret-bindings");
      await expect(reloaded.getByTestId("repository-secret-key-0")).toHaveValue("E2E_MOBILE_TOKEN");
      await expect(reloaded.getByTestId("repository-secret-select-0")).toContainText(
        "E2E Mobile Workspace Secret",
      );
      await assertNoDocumentHorizontalOverflow(testPage, "reloaded mobile repository secrets");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.deleteSecretIfPresent(workspaceSecret.id, seedData.workspaceId);
    }
  });
});
