import { test, expect } from "../../fixtures/test-base";
import {
  exerciseComposer,
  expectActualGeometry,
  openInspection,
  touchScroll,
} from "./image-inspection-helpers";

// @covers AC-AGENTS-SHARED-IMAGE-INSPECTION-001.2, AC-AGENTS-SHARED-IMAGE-INSPECTION-001.4
test("phone inspection contains touch scrolling and controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const { trigger, dialog, image } = await openInspection(testPage, apiClient, seedData, true);
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dialog.getByRole("button", { name: "Actual size", exact: true }).tap();
  const canvas = await expectActualGeometry(dialog);
  await touchScroll(testPage, canvas, "x");
  await touchScroll(testPage, canvas, "y");
  for (const axis of ["x", "y"] as const) {
    for (let i = 0; i < 20; i++) {
      const remaining = await canvas.evaluate(
        (el, key) =>
          key === "x"
            ? el.scrollWidth - el.clientWidth - el.scrollLeft
            : el.scrollHeight - el.clientHeight - el.scrollTop,
        axis,
      );
      if (remaining <= 1) break;
      await touchScroll(testPage, canvas, axis);
    }
    await expect
      .poll(() =>
        canvas.evaluate(
          (el, key) =>
            key === "x"
              ? el.scrollWidth - el.clientWidth - el.scrollLeft
              : el.scrollHeight - el.clientHeight - el.scrollTop,
          axis,
        ),
      )
      .toBeLessThanOrEqual(1);
  }
  const viewport = testPage.viewportSize()!;
  for (const control of [
    dialog.getByRole("button", { name: "Fit", exact: true }),
    dialog.getByRole("button", { name: "Actual size", exact: true }),
    dialog.getByRole("button", { name: "Close", exact: true }),
  ]) {
    const box = (await control.boundingBox())!;
    expect(box.width).toBeGreaterThanOrEqual(44);
    expect(box.height).toBeGreaterThanOrEqual(44);
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
  }
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await prCapture.screenshot("phone-actual-size", {
    caption: "Touch image inspection at actual size",
  });
  await dialog.getByRole("button", { name: "Fit", exact: true }).tap();
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect.poll(async () => (await image.boundingBox())!.width).toBeLessThan(2400);
  await dialog.getByRole("button", { name: "Close", exact: true }).tap();
  await expect(trigger).toBeFocused();
  await trigger.tap();
  await expect(dialog.getByRole("button", { name: "Fit", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await dialog.getByRole("button", { name: "Close", exact: true }).tap();
  await exerciseComposer(testPage, seedData.repositoryPath, true);
});
