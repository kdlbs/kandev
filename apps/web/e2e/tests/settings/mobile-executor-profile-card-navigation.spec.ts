import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  createFixtureExecutorProfile,
  installFixtureExecutorProvider,
} from "../../helpers/plugin-executor-profile";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";

useRegularMode();

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.28
test("phone opens and independently deletes an existing provider profile", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  let executorId = "";
  let selectedId = "";
  let siblingId = "";
  try {
    const executor = await installFixtureExecutorProvider(testPage, apiClient);
    executorId = executor.id;
    await testPage.goto("/settings/executors");
    const selected = await createFixtureExecutorProfile(testPage, executor, true);
    selectedId = selected.id;
    await testPage.goto("/settings/executors");
    const sibling = await createFixtureExecutorProfile(testPage, executor, true);
    siblingId = sibling.id;
    await testPage.goto("/settings/executors");
    const row = testPage.getByTestId(`executor-profile-card-${selected.id}`);
    const open = row.getByRole("button", { name: selected.name });
    const remove = row.getByTestId("executor-profile-delete-button");
    await expect(row).toBeVisible();
    const primaryBox = await requireBox(open, "profile navigation");
    const deleteBox = await requireBox(remove, "profile deletion");
    for (const box of [primaryBox, deleteBox]) {
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    expect(primaryBox.x + primaryBox.width).toBeLessThanOrEqual(deleteBox.x);
    await assertNoDocumentHorizontalOverflow(testPage, "profile card");
    await row.screenshot({ path: test.info().outputPath("profile-card-phone.png") });
    await open.tap();
    await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(selected.id)}`);
    await expect(testPage.getByTestId("plugin-executor-profile-page")).toBeVisible();
    await testPage.goto("/settings/executors");
    const deleted = waitForHttp(
      testPage,
      "DELETE",
      new RegExp(`/api/v1/executors/${executor.id}/profiles/${selected.id}$`),
      { predicate: (response) => response.ok() },
    );
    await remove.tap();
    await deleted;
    await expect(row).toHaveCount(0);
    await expect(testPage).toHaveURL(/\/settings\/executors$/);
    expect(
      (await apiClient.rawRequest("GET", `/api/v1/executors/${executor.id}/profiles/${sibling.id}`))
        .ok,
    ).toBe(true);
  } finally {
    if (executorId && selectedId) {
      await apiClient.rawRequest(
        "DELETE",
        `/api/v1/executors/${executorId}/profiles/${selectedId}`,
      );
    }
    if (executorId && siblingId) {
      await apiClient.rawRequest("DELETE", `/api/v1/executors/${executorId}/profiles/${siblingId}`);
    }
    await uninstallFixturePlugin(apiClient);
  }
});
