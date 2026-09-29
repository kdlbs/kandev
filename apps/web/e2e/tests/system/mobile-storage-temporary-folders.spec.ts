import { test, expect } from "../../fixtures/test-base";
import {
  mockStorageDiskCapacity,
  mockPartialSystemTemporaryOverview,
  mockTemporaryArtifactOverview,
  mockTemporaryEntryBreakdown,
  seedSystemTemporaryFile,
} from "../../helpers/storage-maintenance";

test.describe("Mobile system temporary folders", () => {
  test("shows temporary capacity warnings and keeps the details action touchable", async ({
    testPage,
    prCapture,
  }) => {
    await mockStorageDiskCapacity(testPage);
    await testPage.goto("/settings/system/storage");

    const home = testPage.getByTestId("storage-disk-capacity-card");
    await expect(home).toHaveAttribute("data-severity", "normal");
    const temporary = testPage.getByTestId("storage-temporary-disk-capacity-0");
    await expect(temporary).toHaveAttribute("data-severity", "critical");
    await expect(temporary).toContainText("Temporary-file operations can fail");
    const viewEntries = testPage.getByTestId("storage-disk-view-temporary");
    const box = await viewEntries.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await temporary.scrollIntoViewIfNeeded();
    await prCapture.screenshot("mobile-system-temporary-capacity-warning", {
      caption:
        "Mobile Storage keeps the temporary filesystem warning visible before analysis details",
    });
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);

    const trigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await expect(trigger).toBeVisible();
    await viewEntries.tap();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  test("keeps partial root details readable without horizontal overflow", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    const fixture = seedSystemTemporaryFile(backend.tmpDir, "mobile-partial.txt");
    await mockPartialSystemTemporaryOverview(testPage, fixture.root);
    await testPage.goto("/settings/system/storage");

    const trigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await trigger.tap();
    const resource = testPage.getByTestId("storage-resource-system-temporary");
    await expect(resource).toContainText("Partial");
    await expect(resource).toContainText("Scan timed out. Showing partial usage.");
    await expect(resource).not.toContainText("context deadline exceeded");
    await expect(resource).toContainText(fixture.root);
    await expect(resource).toContainText(
      "Read-only. This footprint can overlap counted categories.",
    );
    await prCapture.screenshot("mobile-system-temporary-partial", {
      caption: "Mobile storage keeps partial temporary-folder details inline",
    });
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  });

  test("shows entry ownership and reviews registered cleanup in a touch-sized flow", async ({
    testPage,
    prCapture,
  }) => {
    await mockTemporaryEntryBreakdown(testPage);
    await testPage.goto("/settings/system/storage");
    const temporaryTrigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await temporaryTrigger.tap();
    const entries = testPage.getByTestId("storage-temporary-entries");
    await expect(entries).toContainText("build-output");
    await expect(entries).toContainText("Not tracked by Kandev");
    await expect(entries).toContainText("Unscanned usage is unknown");
    const review = testPage.getByTestId("storage-temporary-review-cleanup");
    const box = await review.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await prCapture.screenshot("mobile-system-temporary-largest-entries", {
      caption: "Mobile Storage keeps entry names, ownership, and cleanup review readable inline",
    });
    await review.tap();
    const cleanupTrigger = testPage.getByTestId("storage-resource-temporary-artifacts-trigger");
    await expect(cleanupTrigger).toHaveAttribute("aria-expanded", "true");
    await expect
      .poll(() =>
        testPage.evaluate(() => (document.activeElement as HTMLElement | null)?.dataset.testid),
      )
      .toBe("storage-resource-temporary-artifacts-trigger");
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  });

  test("keeps policy persistence and explicit cleanup reachable by touch", async ({
    testPage,
    prCapture,
  }) => {
    await mockTemporaryArtifactOverview(testPage);
    await testPage.route("**/api/v1/system/storage/run", async (route) => {
      expect(route.request().postDataJSON()).toEqual({ resources: ["temporary_artifacts"] });
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "mobile-temporary-artifacts-policy-cleanup" }),
      });
    });
    await testPage.route("**/api/v1/system/jobs/**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "mobile-temporary-artifacts-policy-cleanup",
          kind: "storage-cleanup",
          state: "succeeded",
          started_at: new Date().toISOString(),
        }),
      });
    });

    await testPage.goto("/settings/system/storage");
    const toggleTarget = testPage.getByTestId("storage-temporary-artifacts-enabled-target");
    const toggleBox = await toggleTarget.boundingBox();
    expect(toggleBox).not.toBeNull();
    expect(toggleBox!.width).toBeGreaterThanOrEqual(44);
    expect(toggleBox!.height).toBeGreaterThanOrEqual(44);
    await toggleTarget.tap({ position: { x: 2, y: 2 } });
    await testPage.getByRole("button", { name: "Save changes" }).tap();
    await expect(testPage.getByText("Storage policy saved")).toBeVisible();
    await testPage.reload();
    await expect(testPage.getByTestId("storage-temporary-artifacts-enabled")).toHaveAttribute(
      "aria-checked",
      "true",
    );

    const cleanButton = testPage.getByTestId("storage-policy-temporary-artifacts-clean");
    await expect(cleanButton).toBeVisible();
    const box = await cleanButton.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await cleanButton.tap();
    await expect(testPage.getByText("Clean inactive Kandev temporary files?")).toBeVisible();
    await prCapture.screenshot("mobile-system-temporary-cleanup-confirmation", {
      caption: "Mobile storage keeps explicit registered cleanup in a touch-sized flow",
    });
    await testPage.getByTestId("storage-temporary-artifacts-confirm").tap();
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 30_000 },
    );
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  });
});
