import { test, expect } from "../../fixtures/test-base";
import {
  analyzeStorageAndWait,
  assertWorkspaceStorageDiscoveryFixtureIsUnchanged,
  readWorkspaceStorageSummary,
  seedWorkspaceStorageBaseline,
  seedWorkspaceStorageDiscovery,
} from "../../helpers/workspace-storage-discovery";

test.describe("Workspace storage discovery", () => {
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
  test("measures a marked workspace beside an unrelated checkout", async ({
    testPage,
    backend,
  }) => {
    const baselineFixture = seedWorkspaceStorageBaseline(backend.tmpDir);
    try {
      await testPage.goto("/settings/system/storage");
      await expect(testPage.getByTestId("storage-overview-card")).toBeVisible();

      await analyzeStorageAndWait(testPage, "click");
      const baseline = await readWorkspaceStorageSummary(testPage);
      expect(baseline.total_bytes).toBeGreaterThanOrEqual(baselineFixture.expectedBytes);

      const fixture = seedWorkspaceStorageDiscovery(backend.tmpDir);
      try {
        await analyzeStorageAndWait(testPage, "click");
        const workspaces = await readWorkspaceStorageSummary(testPage);
        expect(workspaces.total_bytes).toBe(baseline.total_bytes + fixture.expectedBytes);
        expect(workspaces.warnings).toContain(
          `unclassified task directory kept: ${fixture.checkoutRoot}`,
        );

        const workspaceResource = testPage.getByTestId("storage-resource-workspaces");
        const workspaceTrigger = testPage.getByTestId("storage-resource-workspaces-trigger");
        await expect(workspaceTrigger).toContainText("Task workspaces");
        await expect(workspaceTrigger).toContainText("GB");
        await workspaceTrigger.click();
        await expect(workspaceTrigger).toHaveAttribute("aria-expanded", "true");
        await expect(workspaceResource).toContainText("Task workspaces");
        assertWorkspaceStorageDiscoveryFixtureIsUnchanged(fixture);
      } finally {
        fixture.cleanup();
      }
    } finally {
      baselineFixture.cleanup();
    }
  });
});
