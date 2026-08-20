import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile workspace repository sets", () => {
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
