import { test, expect } from "../../fixtures/test-base";
import {
  assertWorkspaceStorageDiscoveryFixtureIsUnchanged,
  seedWorkspaceStorageDiscovery,
} from "../../helpers/workspace-storage-discovery";

test.describe("Mobile workspace storage discovery", () => {
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
  // @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
  test("measures a marked workspace beside an unrelated checkout by touch", async ({
    testPage,
    backend,
  }) => {
    const fixture = seedWorkspaceStorageDiscovery(backend.tmpDir);
    try {
      await testPage.goto("/settings/system/storage");
      await expect(testPage.getByTestId("storage-overview-card")).toBeVisible();

      await testPage.getByTestId("storage-analyze").tap();
      await expect(testPage.getByTestId("storage-analyze")).toHaveAttribute(
        "data-job-state",
        "succeeded",
      );

      const workspaces = await testPage.evaluate(async () => {
        const response = await fetch("/api/v1/system/storage");
        if (!response.ok) throw new Error(`Storage overview failed: ${response.status}`);
        const overview = await response.json();
        return overview.summary.workspaces;
      });
      expect(workspaces.total_bytes).toBe(fixture.expectedBytes);
      expect(workspaces.warnings).toContain(
        `unclassified task directory kept: ${fixture.checkoutRoot}`,
      );

      const workspaceResource = testPage.getByTestId("storage-resource-workspaces");
      const workspaceTrigger = testPage.getByTestId("storage-resource-workspaces-trigger");
      await expect(workspaceTrigger).toContainText("Task workspaces");
      await expect(workspaceTrigger).toContainText("<0.01 GB");
      await workspaceTrigger.tap();
      await expect(workspaceTrigger).toHaveAttribute("aria-expanded", "true");
      await expect(workspaceResource).toContainText("Task workspaces");
      await expect
        .poll(() =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);
      assertWorkspaceStorageDiscoveryFixtureIsUnchanged(fixture);
    } finally {
      fixture.cleanup();
    }
  });
});
