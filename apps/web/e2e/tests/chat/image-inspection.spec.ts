import { test, expect } from "../../fixtures/test-base";
import { exerciseComposer, expectActualGeometry, openInspection } from "./image-inspection-helpers";
import { seedSizedImage } from "./rich-output-image-helpers";
import { RICH_OUTPUT_IMAGE, seedRichOutputTask } from "./rich-output-helpers";

// @covers AC-AGENTS-SHARED-IMAGE-INSPECTION-001.1, AC-AGENTS-SHARED-IMAGE-INSPECTION-001.2
test("desktop inspection uses actual pixels and keyboard scrolling", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { trigger, dialog, image, source } = await openInspection(
    testPage,
    apiClient,
    seedData,
    false,
  );
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dialog.getByRole("button", { name: "Actual size", exact: true }).click();
  const canvas = await expectActualGeometry(dialog);
  await expect(image).toHaveAttribute("src", source!);
  await canvas.focus();
  await testPage.keyboard.press("ArrowRight");
  await testPage.keyboard.press("ArrowDown");
  await expect.poll(() => canvas.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  await expect.poll(() => canvas.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  for (let i = 0; i < 100; i++) await testPage.keyboard.press("ArrowRight");
  await testPage.keyboard.press("Control+End");
  await expect
    .poll(() => canvas.evaluate((el) => el.scrollWidth - el.clientWidth - el.scrollLeft))
    .toBeLessThanOrEqual(1);
  await expect
    .poll(() => canvas.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
    .toBeLessThanOrEqual(1);
  await prCapture.screenshot("desktop-actual-size", { caption: "Actual-size image inspection" });
  await dialog.getByRole("button", { name: "Fit", exact: true }).click();
  expect((await image.boundingBox())!.width).toBeLessThan(2400);
  await testPage.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await testPage.keyboard.press("Enter");
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await testPage.keyboard.press("Escape");
  await exerciseComposer(testPage, seedData.repositoryPath, false);
});

test("small images remain contained at a narrow fine-pointer viewport", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 700, height: 800 });
  const payload = await seedSizedImage(testPage, seedData.repositoryPath, {
    width: 20,
    height: 20,
  });
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Small image inspection",
    payload,
  });
  const trigger = session
    .activeChat()
    .getByRole("button", { name: `Open ${RICH_OUTPUT_IMAGE}`, exact: true });
  await trigger.click();
  const dialog = testPage.getByRole("dialog", { name: "Image preview" });
  await dialog.getByRole("button", { name: "Actual size", exact: true }).click();
  const region = dialog.getByRole("region", { name: "Image inspection" });
  await expect
    .poll(() =>
      region.evaluate(
        (el) => el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight,
      ),
    )
    .toBe(true);
  const size = await dialog.boundingBox();
  expect(size!.x).toBeGreaterThanOrEqual(0);
  expect(size!.x + size!.width).toBeLessThanOrEqual(700);
  await testPage.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});
