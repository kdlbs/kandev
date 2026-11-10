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
    const chevron = row.locator("svg").last().locator("..");
    await expect(row).toBeVisible();
    await row.scrollIntoViewIfNeeded();
    const primaryBox = await requireBox(open, "profile navigation");
    const deleteBox = await requireBox(remove, "profile deletion");
    const chevronBox = await requireBox(chevron, "profile chevron target");
    for (const box of [primaryBox, deleteBox, chevronBox]) {
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    expect(primaryBox.x + primaryBox.width).toBeLessThanOrEqual(deleteBox.x);
    expect(deleteBox.x + deleteBox.width).toBeLessThanOrEqual(chevronBox.x);
    await assertNoDocumentHorizontalOverflow(testPage, "profile card");
    await row.screenshot({ path: test.info().outputPath("profile-card-phone.png") });
    const deletes: string[] = [];
    testPage.on("request", (request) => {
      if (request.method() === "DELETE" && request.url().endsWith(`/profiles/${selected.id}`)) {
        deletes.push(request.url());
      }
    });
    const rowBox = await requireBox(row, "profile row");
    await testPage.touchscreen.tap(rowBox.x + rowBox.width - 20, rowBox.y + rowBox.height / 2);
    await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(selected.id)}`);
    expect(deletes).toEqual([]);
    await testPage.goto("/settings/executors");
    await row.scrollIntoViewIfNeeded();
    const paddingBox = await requireBox(row, "profile row padding");
    await testPage.touchscreen.tap(paddingBox.x + 4, paddingBox.y + paddingBox.height / 2);
    await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(selected.id)}`);
    expect(deletes).toEqual([]);
    await testPage.goto("/settings/executors");
    await open.tap();
    await expect(testPage).toHaveURL(`/settings/executors/${encodeURIComponent(selected.id)}`);
    await expect(testPage.getByTestId("plugin-executor-profile-page")).toBeVisible();
    expect(deletes).toEqual([]);
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
