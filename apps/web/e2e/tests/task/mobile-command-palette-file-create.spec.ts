import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  createControl,
  duplicateFailure,
  openCreateFile,
  preparePaletteFile,
} from "./palette-file-create-helpers";

// @covers AC-UI-PALETTE-FILE-CREATE-001.1, AC-UI-PALETTE-FILE-CREATE-001.3, AC-UI-PALETTE-FILE-CREATE-001.4
test("phone palette reports duplicate file creation", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const fixture = await preparePaletteFile(testPage, apiClient, seedData);
  await createControl(testPage, fixture, true);
  // The existing phone layout remains on chat after the desktop editor-panel request.
  await expect(testPage.getByTestId("chat-input-editor")).toBeVisible();
  await testPage.screenshot({ path: testInfo.outputPath("phone-success.png") });
  const notice = await duplicateFailure(testPage, fixture, true);
  await waitForFiniteAnimations(notice);
  const box = await notice.boundingBox();
  const viewport = testPage.viewportSize();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: testInfo.outputPath("phone-error.png") });
  const { palette } = await openCreateFile(testPage, true);
  const close = palette.getByRole("button", { name: "Close", exact: true });
  expect((await close.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await close.tap();
  await expect(palette).toBeHidden();
});
