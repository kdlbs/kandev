import path from "node:path";
import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

// AC-37/38: mirrors the desktop happy-path coverage in
// automations-export.spec.ts, but taps the control on a coarse-pointer
// viewport and asserts the touch target itself — the button's mobile
// `min-h-11` class (components/automations/automations-export-button.tsx)
// must resolve to a real ~44px tap target, not just look right in markup.

test.describe("Automations export on mobile", () => {
  test("export control meets the mobile touch target and downloads on tap", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();
    await expect(automations.emptyState).toBeVisible({ timeout: 10_000 });
    await expect(automations.exportButton).toBeEnabled();
    await expect(automations.newAutomationButton).toBeVisible();

    const [exportBox, newAutomationBox] = await Promise.all([
      automations.exportButton.boundingBox(),
      automations.newAutomationButton.boundingBox(),
    ]);
    expect(exportBox).not.toBeNull();
    expect(newAutomationBox).not.toBeNull();
    expect(exportBox!.height).toBeGreaterThanOrEqual(44);
    expect(newAutomationBox!.height).toBeCloseTo(exportBox!.height, 1);

    const downloadPromise = testPage.waitForEvent("download");
    await automations.exportButton.tap();
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toBe("kandev-automations.zip");
    await download.saveAs(path.join(backend.tmpDir, `export-mobile-${Date.now()}.zip`));
  });
});
