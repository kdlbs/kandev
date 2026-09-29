import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  mockStorageDiskCapacity,
  mockPartialSystemTemporaryOverview,
  mockTemporaryArtifactOverview,
  mockTemporaryEntryBreakdown,
  seedSystemTemporaryFile,
} from "../../helpers/storage-maintenance";

test.describe("System temporary folders", () => {
  test("shows critical temporary capacity before folder analysis completes", async ({
    testPage,
    prCapture,
  }) => {
    await mockStorageDiskCapacity(testPage);
    let releaseOverview!: () => void;
    let markOverviewStarted!: () => void;
    const overviewReleased = new Promise<void>((resolve) => {
      releaseOverview = resolve;
    });
    const overviewStarted = new Promise<void>((resolve) => {
      markOverviewStarted = resolve;
    });
    await testPage.route("**/api/v1/system/storage", async (route) => {
      if (route.request().method() === "GET") {
        markOverviewStarted();
        await overviewReleased;
      }
      await route.continue();
    });

    await testPage.goto("/settings/system/storage");
    await overviewStarted;
    const home = testPage.getByTestId("storage-disk-capacity-card");
    await expect(home).toHaveAttribute("data-severity", "normal");
    const temporary = testPage.getByTestId("storage-temporary-disk-capacity-0");
    await expect(temporary).toHaveAttribute("data-severity", "critical");
    await expect(temporary).toContainText("/isolated/tmp");
    await expect(temporary).toContainText(
      "Temporary-file operations can fail when this filesystem is full",
    );
    await prCapture.screenshot("system-temporary-capacity-warning", {
      caption: "Storage reports critical temporary capacity while folder analysis is pending",
    });

    releaseOverview();
    const trigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await expect(trigger).toBeVisible();
    await testPage.getByTestId("storage-disk-view-temporary").click();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

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
    await expect(resource).toContainText("Scan timed out. Showing partial usage.");
    await expect(resource).not.toContainText("context deadline exceeded");
    await expect(resource).toContainText(fixture.root);
    await expect(resource).toContainText("One fixture entry was skipped.");
    await prCapture.screenshot("system-temporary-partial", {
      caption: "Desktop storage explains a partial system temporary measurement",
    });
  });

  test("shows bounded entry ownership and navigates to existing cleanup without running it", async ({
    testPage,
    prCapture,
  }) => {
    await mockTemporaryEntryBreakdown(testPage);
    const storageMutations: string[] = [];
    testPage.on("request", (request) => {
      if (request.method() !== "GET" && request.url().includes("/api/v1/system/storage")) {
        storageMutations.push(request.url());
      }
    });
    await testPage.goto("/settings/system/storage");

    const temporaryTrigger = testPage.getByTestId("storage-resource-system-temporary-trigger");
    await expect(temporaryTrigger).toBeVisible();
    await temporaryTrigger.click();
    const entries = testPage.getByTestId("storage-temporary-entries");
    await expect(entries).toContainText("build-output");
    await expect(entries).toContainText("900,000,000 B");
    await expect(entries).toContainText("Not tracked by Kandev");
    await expect(entries).toContainText("active-profile");
    await expect(entries).toContainText("Ownership unknown");
    await expect(entries).toContainText("5 other observed entries");
    await expect(entries).toContainText("1 stale candidate");
    await prCapture.screenshot("system-temporary-largest-entries", {
      caption: "Storage labels measured temporary entries and explains registered cleanup scope",
    });

    await testPage.getByTestId("storage-temporary-review-cleanup").click();
    const cleanupTrigger = testPage.getByTestId("storage-resource-temporary-artifacts-trigger");
    await expect(cleanupTrigger).toHaveAttribute("aria-expanded", "true");
    await expect
      .poll(() =>
        testPage.evaluate(() => (document.activeElement as HTMLElement | null)?.dataset.testid),
      )
      .toBe("storage-resource-temporary-artifacts-trigger");
    expect(storageMutations).toEqual([]);
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
    await expect(testPage.getByText("Clean inactive Kandev temporary files?")).toBeVisible();
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
