import { test, expect } from "../../fixtures/test-base";
import {
  mockPartialSystemTemporaryOverview,
  mockTemporaryArtifactOverview,
  seedSystemTemporaryFile,
} from "../../helpers/storage-maintenance";

test.describe("Mobile system temporary folders", () => {
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
    await testPage.getByTestId("storage-temporary-artifacts-enabled").tap();
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
    await expect(testPage.getByText("Clean stale Kandev artifacts?")).toBeVisible();
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
