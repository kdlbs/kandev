import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { createControl, duplicateFailure, preparePaletteFile } from "./palette-file-create-helpers";

// @covers AC-UI-PALETTE-FILE-CREATE-001.1, AC-UI-PALETTE-FILE-CREATE-001.2, AC-UI-PALETTE-FILE-CREATE-001.4
test("creates a file and reports a duplicate without overwriting it", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const fixture = await preparePaletteFile(testPage, apiClient, seedData);
  await test.step("ordinary creation opens the editor", async () => {
    await createControl(testPage, fixture, false);
    await expect(
      testPage.getByText("palette-create-check.txt", { exact: true }).first(),
    ).toBeVisible();
  });
  await test.step("duplicate creation visibly reports the real rejection", async () => {
    const notice = await duplicateFailure(testPage, fixture, false);
    await waitForFiniteAnimations(notice);
    await testPage.screenshot({ path: testInfo.outputPath("desktop-error.png") });
  });
});
