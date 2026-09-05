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
  test("keeps the create-grant dialog contained and its submit action reachable", async ({
    testPage,
    backend,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR_TASK_AUTHORITY: "true",
    });

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

      await testPage
        .getByTestId("grant-task-id-input")
        .fill("00000000-0000-4000-8000-000000000001");
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
    } finally {
      await releaseFeature();
    }
  });
});
