import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Mobile workspace repository sets", () => {
  test("opens the inline editor as a contained full-height drawer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const setName = `Mobile editor set ${Date.now()}`;
    const created = await apiClient.createRepositorySet(seedData.workspaceId, setName, [
      seedData.repositoryId,
    ]);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await testPage.getByTestId(`repository-set-edit-${created.id}`).tap();

    const surface = testPage.getByTestId("repository-set-editor-surface");
    await expect(surface).toBeVisible();
    await expect(surface).toHaveClass(/h-\[100dvh\]/);
    await expect(testPage.getByTestId("repository-set-editor-form")).toHaveClass(
      /min-h-0.*overflow-y-auto/,
    );
    await prCapture.screenshot("mobile-repository-set-editor", {
      caption:
        "The mobile repository set editor uses a full-height drawer with a fixed action bar.",
    });

    for (const control of [
      testPage.getByTestId("repository-set-editor-save"),
      testPage.getByTestId("repository-set-editor-cancel"),
      testPage.getByTestId(`repository-set-base-${seedData.repositoryId}`),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    execSync("git update-ref refs/remotes/origin/main HEAD", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    const basePicker = testPage.getByTestId(`repository-set-base-${seedData.repositoryId}`);
    await basePicker.tap();
    const dropdown = testPage.getByTestId(`repository-set-base-dropdown-${seedData.repositoryId}`);
    await expect(dropdown).toBeVisible();
    await expect(dropdown.getByPlaceholder("Search branches...")).toBeVisible();
    await expect(dropdown.getByText("origin/main")).toBeVisible();
    await expect(dropdown.getByText("origin", { exact: true })).toBeVisible();

    const search = dropdown.getByPlaceholder("Search branches...");
    await search.fill("origin");
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();
    await expect(dropdown.getByRole("option", { name: /^main local/ })).toHaveCount(0);
    await dropdown.getByTestId("branch-refresh-button").tap();
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();

    const dropdownBox = await dropdown.boundingBox();
    const viewport = testPage.viewportSize();
    expect(dropdownBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(dropdownBox!.x).toBeGreaterThanOrEqual(0);
    expect(dropdownBox!.y).toBeGreaterThanOrEqual(0);
    expect(dropdownBox!.x + dropdownBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dropdownBox!.y + dropdownBox!.height).toBeLessThanOrEqual(viewport!.height);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile repository-set editor");
  });

  test("confirms deletion inline without a second overlay", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const setName = `Mobile settings set ${Date.now()}`;
    const created = await apiClient.createRepositorySet(seedData.workspaceId, setName, [
      seedData.repositoryId,
    ]);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const row = testPage.getByTestId("repository-set-row");
    await expect(row).toContainText(setName);

    await row.getByTestId(`repository-set-delete-${created.id}`).tap();
    const inline = row.getByTestId("repository-set-delete-inline-confirmation");
    await expect(inline).toBeVisible();
    await expect(inline).toContainText(
      "The set is removed. Its repositories, and any task already using them, are not affected.",
    );
    await expect(testPage.getByTestId("repository-set-delete-confirm-popover")).toHaveCount(0);

    for (const control of [
      inline.getByTestId("repository-set-delete-confirm"),
      inline.getByRole("button", { name: "Cancel" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile repository-set confirmation");

    await inline.getByTestId("repository-set-delete-confirm").tap();
    await expect(testPage.getByTestId("repository-sets-empty")).toBeVisible();
    await expect
      .poll(async () => {
        const listed = await apiClient.listRepositorySets(seedData.workspaceId);
        return listed.repository_sets.some((entry) => entry.id === created.id);
      })
      .toBe(false);
  });
});
