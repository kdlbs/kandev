import { test, expect } from "../../fixtures/test-base";

/**
 * Office run-history retention (docs/specs/office/requirements/run-history-retention*.md).
 * The sweep itself is time- and seed-dependent and is covered far more cheaply by backend
 * tests; the two honest E2E candidates are the settings round-trip through the real API
 * and a threshold warning actually reaching the Health card. Both tests restore whatever
 * global retention settings they change, since retention settings are process-global and
 * this worker's backend is reused by every test file in the shard.
 */
test.describe("System retention settings", () => {
  test("persists an admin policy edit through save and across a backend restart", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(90_000);
    await testPage.goto("/settings/system/data-storage");
    const batchLimit = testPage.getByTestId("retention-batch-limit");
    await expect(batchLimit).toBeVisible();
    const original = await batchLimit.inputValue();
    const updated = original === "7777" ? "8888" : "7777";

    try {
      await batchLimit.fill(updated);
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByTestId("settings-floating-save")).toContainText("Saved");

      await testPage.reload();
      await expect(testPage.getByTestId("retention-batch-limit")).toHaveValue(updated);

      await backend.restart();
      await testPage.reload();
      await expect(testPage.getByTestId("retention-batch-limit")).toHaveValue(updated);
    } finally {
      await testPage.getByTestId("retention-batch-limit").fill(original);
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByTestId("settings-floating-save")).toContainText("Saved");
    }
  });

  test("shows a threshold warning on the Health card once retained runs exceed the configured limit", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.goto("/settings/system/data-storage");
    const warnField = testPage.getByTestId("retention-runs-warn-rows");
    await expect(warnField).toBeVisible();
    const originalWarnRows = await warnField.inputValue();

    const initialStatus = await testPage.evaluate(async () => {
      const response = await fetch("/api/v1/system/retention");
      return response.json();
    });
    const baselineRetained =
      initialStatus.retained_counts.runs.state === "fresh"
        ? initialStatus.retained_counts.runs.retained_count
        : 0;
    // warn_rows=0 disables the threshold check entirely (AC-OFFICE-RUN-HISTORY-RETENTION-004.3),
    // and seeding 3 new terminal runs guarantees the post-seed count clears baseline+1 even when
    // baseline is 0.
    const seededRunCount = 3;
    const newWarnRows = baselineRetained + 1;
    const expectedRetained = baselineRetained + seededRunCount;
    for (let i = 0; i < seededRunCount; i++) {
      await apiClient.seedRun({ agentProfileId: seedData.agentProfileId, status: "finished" });
    }

    try {
      await warnField.fill(String(newWarnRows));
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByTestId("settings-floating-save")).toContainText("Saved");

      // The census only re-evaluates on its interval timer or at scheduler Start; a
      // restart forces an immediate re-evaluation against the settings just saved and
      // the runs just seeded, deterministically, without waiting out the real interval.
      await backend.restart();

      await testPage.goto("/settings/system/status");
      const issue = testPage.getByTestId("system-health-issue-office_retention_threshold:runs");
      await expect(issue).toBeVisible({ timeout: 15_000 });
      await expect(issue).toContainText("over its threshold");

      await testPage.goto("/settings/system/data-storage");
      const retainedRuns = testPage.getByTestId("retention-retained-runs");
      await expect(retainedRuns).toBeVisible();
      await expect(retainedRuns).toContainText(String(expectedRetained));
    } finally {
      await testPage.goto("/settings/system/data-storage");
      await testPage.getByTestId("retention-runs-warn-rows").fill(originalWarnRows);
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByTestId("settings-floating-save")).toContainText("Saved");
    }
  });
});
