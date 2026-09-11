import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  assertNoDocumentHorizontalOverflow,
  assertNoElementHorizontalOverflow,
} from "../../helpers/layout-assertions";

async function expectDialogContained(dialog: Locator) {
  const metrics = await dialog.evaluate((node: HTMLElement) => {
    const rect = node.getBoundingClientRect();
    return {
      bottom: rect.bottom,
      top: rect.top,
      viewportHeight: window.innerHeight,
    };
  });

  expect(metrics.top, "dialog top should stay inside the mobile viewport").toBeGreaterThanOrEqual(
    0,
  );
  expect(
    metrics.bottom,
    "dialog footer should stay inside the mobile viewport instead of being clipped",
  ).toBeLessThanOrEqual(metrics.viewportHeight);
}

test.describe("Mobile coordinator grants", () => {
  test("creates, persists, and revokes a coordinator grant with a contained mobile dialog", async ({
    testPage,
    backend,
    seedData,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR_TASK_AUTHORITY: "true",
    });
    const coordinatorTask = await apiClient.seedTask(
      seedData.workspaceId,
      "QA Mobile Coordinator Task",
      {
        workflow_id: seedData.workflowId,
      },
    );

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/coordinators`);
      await expect(testPage.getByTestId("coordinator-grants-page")).toBeVisible({
        timeout: 15_000,
      });

      await testPage.getByTestId("create-grant-button").tap();
      const dialog = testPage.getByRole("dialog");
      await expect(dialog).toBeVisible();

      await expectDialogContained(dialog);
      await assertNoElementHorizontalOverflow(dialog, "mobile coordinator grant dialog");
      await assertNoDocumentHorizontalOverflow(testPage, "mobile coordinator grant dialog");

      await testPage.getByTestId("grant-task-id-input").fill(coordinatorTask.task_id);
      await testPage.getByTestId("grant-cap-inspect").tap();

      const submit = testPage.getByTestId("grant-create-submit");
      await expect(submit).toBeVisible();
      const submitBox = await submit.boundingBox();
      expect(submitBox, "create grant submit has no rendered hitbox").not.toBeNull();
      expect(submitBox?.height ?? 0).toBeGreaterThanOrEqual(44);

      const submitHitTarget = await submit.evaluate((node) => {
        const rect = node.getBoundingClientRect();
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2,
        );
        return hit === node || node.contains(hit);
      });
      expect(submitHitTarget, "create grant submit center should be tappable").toBe(true);

      await testPage.addStyleTag({
        content: '[data-testid="toast-container"] { display: none !important; }',
      });
      await prCapture.screenshot("mobile-coordinator-grants-create-dialog", {
        caption: "Mobile coordinator grant creation dialog with contained scrollable form",
      });

      await submit.tap();
      await expect(dialog).toBeHidden();

      const grantRow = testPage
        .locator('[data-testid^="grant-row-"]')
        .filter({ hasText: coordinatorTask.task_id });
      await expect(grantRow).toBeVisible();
      await testPage.reload();
      await expect(grantRow).toBeVisible();

      await grantRow.locator('[data-testid^="revoke-grant-"]').tap();
      await testPage.getByTestId("revoke-grant-confirm").tap();
      await expect(grantRow).toBeHidden();
      await testPage.reload();
      await expect(grantRow).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });
});
