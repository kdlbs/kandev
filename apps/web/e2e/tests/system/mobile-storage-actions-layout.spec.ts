import { test, expect } from "../../fixtures/test-base";
import {
  expectStorageActionHitTarget,
  expectStorageActionsFit,
  expectStorageActionStatusNodesRetained,
  markStorageActionStatusNodes,
  mockStorageActionJobs,
} from "../../helpers/storage-actions-layout";
import { setStoreRole } from "../../helpers/session-store";

// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.4
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.7
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.10
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.11
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.12
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.13
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.14
test("stacks actions with usable touch targets", async ({ testPage, prCapture }) => {
  await testPage.emulateMedia({ colorScheme: "dark" });
  const jobs = await mockStorageActionJobs(testPage);
  await testPage.goto("/settings/system/storage");
  await expectStorageActionsFit(testPage);
  await markStorageActionStatusNodes(testPage);
  await prCapture.screenshot("phone", {
    caption: "Phone Storage places Analyze and Run now in separate full-width rows",
  });

  for (const width of [320, 767]) {
    await testPage.setViewportSize({ width, height: 900 });
    await expectStorageActionsFit(testPage);
    await expectStorageActionHitTarget(testPage, "storage-analyze");
    await expectStorageActionHitTarget(testPage, "storage-run-now");
  }
  await testPage.setViewportSize({ width: 320, height: 900 });
  jobs.setState("queued");
  await testPage.getByTestId("storage-analyze").tap();
  await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute("data-job-state", "queued");
  await testPage.getByTestId("storage-run-now").tap();
  await expect(testPage.getByTestId("storage-run-now")).toHaveAttribute("data-job-state", "queued");
  await expect(testPage.getByTestId("storage-analyze")).toHaveAccessibleName("Analyze");
  await expect(testPage.getByTestId("storage-analyze-control").getByRole("status")).toHaveText(
    "Analyzing...",
  );
  await expect(testPage.getByTestId("storage-run-now")).toHaveAccessibleName("Run now");
  await expect(testPage.getByTestId("storage-cleanup-control").getByRole("status")).toHaveText(
    "Cleaning...",
  );
  await testPage.setViewportSize({ width: 393, height: 851 });
  await expectStorageActionsFit(testPage);
  await expectStorageActionStatusNodesRetained(testPage);
  jobs.setState("failed");
  for (const [id, control, label] of [
    ["storage-analyze", "storage-analyze-control", "Analysis failed"],
    ["storage-run-now", "storage-cleanup-control", "Cleanup failed"],
  ]) {
    await expect(testPage.getByTestId(id)).toHaveText(label);
    await expect(testPage.getByTestId(control).getByRole("status")).toHaveText(label);
  }
  await expectStorageActionsFit(testPage);
  await expectStorageActionStatusNodesRetained(testPage);
  jobs.setState("succeeded");
  await testPage.getByTestId("storage-analyze").tap();
  await testPage.getByTestId("storage-run-now").tap();
  await expect(testPage.getByTestId("storage-analyze")).toHaveText("Analysis complete");
  await expect(testPage.getByTestId("storage-run-now")).toHaveText("Cleanup complete");
  await expect(testPage.getByTestId("storage-analyze-control").getByRole("status")).toHaveText(
    "Analysis complete",
  );
  await expect(testPage.getByTestId("storage-cleanup-control").getByRole("status")).toHaveText(
    "Cleanup complete",
  );
  expect(jobs.requests).toEqual({ analyze: 2, run: 2 });
  await expectStorageActionsFit(testPage);
  await expectStorageActionStatusNodesRetained(testPage);
  await testPage.getByTestId("system-page-title").scrollIntoViewIfNeeded();
  await prCapture.screenshot("phone-completed", {
    caption: "Both Storage results stay readable inside phone-sized action rows",
  });
  await setStoreRole(testPage, "member");
  await expect(testPage.getByTestId("storage-analyze")).toBeDisabled();
  await expectStorageActionsFit(testPage);
  await expectStorageActionStatusNodesRetained(testPage);
});

// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.1
// @covers AC-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001.4
test("retains action containment on a coarse-pointer tablet", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 820, height: 1180 });
  await mockStorageActionJobs(testPage);
  await testPage.goto("/settings/system/storage");
  expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  await expectStorageActionsFit(testPage);
  for (const id of ["storage-analyze", "storage-run-now"]) {
    await expectStorageActionHitTarget(testPage, id);
    await testPage.getByTestId(id).tap();
    await expect(testPage.getByTestId(id)).toHaveAttribute("data-job-state", "succeeded");
  }
  await expectStorageActionsFit(testPage);
});
