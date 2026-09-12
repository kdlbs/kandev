import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  mockPartialSystemTemporaryOverview,
  mockTemporaryArtifactOverview,
  seedSystemTemporaryFile,
} from "../../helpers/storage-maintenance";

test.describe("System temporary folders", () => {
  test("measures the disposable root again after Analyze and excludes it from the total", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    const fixture = seedSystemTemporaryFile(backend.tmpDir, "initial.txt");
    await testPage.goto("/settings/system/storage");

    const initial = await testPage.evaluate(async () => {
      const response = await fetch("/api/v1/system/storage");
      return response.json();
    });
    const initialBytes = initial.summary?.system_temporary?.size_bytes ?? 0;
    fs.writeFileSync(`${fixture.root}/after-refresh.txt`, "temporary-refresh-fixture");

    await testPage.getByTestId("storage-analyze").click();
    await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 30_000 },
    );
    await expect
      .poll(async () => {
        const overview = await testPage.evaluate(async () => {
          const response = await fetch("/api/v1/system/storage");
          return response.json();
        });
        return overview.summary?.system_temporary?.size_bytes ?? 0;
      })
      .toBeGreaterThan(initialBytes);

    const trigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await expect(trigger).toContainText("GB");
    await trigger.click();
    await expect(testPage.getByTestId("storage-resource-system-temporary")).toContainText(
      "Read-only. This footprint can overlap counted categories.",
    );
    await expect(testPage.getByTestId("storage-resource-system-temporary")).toContainText(
      fixture.root,
    );
    const refreshed = await testPage.evaluate(async () => {
      const response = await fetch("/api/v1/system/storage");
      return response.json();
    });
    expect(refreshed.summary.system_temporary.included_in_total).toBe(false);
    await prCapture.screenshot("system-temporary-folders", {
      caption:
        "Desktop storage shows the refreshed system temporary footprint outside the counted total",
    });
  });

  test("shows partial measurement details and the informational overlap policy", async ({
    testPage,
    backend,
    prCapture,
  }) => {
    const fixture = seedSystemTemporaryFile(backend.tmpDir, "partial.txt");
    await mockPartialSystemTemporaryOverview(testPage, fixture.root);
    await testPage.goto("/settings/system/storage");

    const trigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await expect(trigger).toContainText("<0.01 GB");
    await trigger.click();
    const resource = testPage.getByTestId("storage-resource-system-temporary");
    await expect(resource).toContainText("Partial");
    await expect(resource).toContainText(fixture.root);
    await expect(resource).toContainText("One fixture entry was skipped.");
    await prCapture.screenshot("system-temporary-partial", {
      caption: "Desktop storage explains a partial system temporary measurement",
    });
  });

  test("persists the opt-in policy and confirms explicit cleanup through quarantine", async ({
    testPage,
    prCapture,
  }) => {
    await mockTemporaryArtifactOverview(testPage);
    await testPage.route("**/api/v1/system/storage/run", async (route) => {
      expect(route.request().method()).toBe("POST");
      expect(route.request().postDataJSON()).toEqual({ resources: ["temporary_artifacts"] });
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ job_id: "temporary-artifacts-policy-cleanup" }),
      });
    });
    await testPage.route("**/api/v1/system/jobs/**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "temporary-artifacts-policy-cleanup",
          kind: "storage-cleanup",
          state: "succeeded",
          started_at: new Date().toISOString(),
        }),
      });
    });

    await testPage.goto("/settings/system/storage");
    await testPage.getByTestId("storage-temporary-artifacts-enabled").click();
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(testPage.getByText("Storage policy saved")).toBeVisible();
    await testPage.reload();
    await expect(testPage.getByTestId("storage-temporary-artifacts-enabled")).toHaveAttribute(
      "aria-checked",
      "true",
    );

    await testPage.getByTestId("storage-policy-temporary-artifacts-clean").click();
    await expect(testPage.getByText("Clean stale Kandev artifacts?")).toBeVisible();
    await prCapture.screenshot("system-temporary-cleanup-confirmation", {
      caption: "Desktop storage confirms registered artifact cleanup before quarantine",
    });
    await testPage.getByTestId("storage-temporary-artifacts-confirm").click();
    await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute(
      "data-job-state",
      "succeeded",
      { timeout: 30_000 },
    );
  });
});
