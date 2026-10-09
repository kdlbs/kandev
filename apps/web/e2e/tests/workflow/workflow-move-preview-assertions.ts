import { expect, type Locator } from "@playwright/test";

export async function expectPreviewFooter(surface: Locator, moveTestId: string, touch = false) {
  const preview = surface.getByTestId("workflow-move-preview");
  await expect(preview).toBeVisible();
  await expect(preview).toContainText("Reuse current session");
  const moveBox = await surface.getByTestId(moveTestId).boundingBox();
  const previewBox = await preview.boundingBox();
  expect(moveBox).not.toBeNull();
  expect(previewBox).not.toBeNull();
  expect(previewBox!.y).toBeGreaterThanOrEqual(moveBox!.y + moveBox!.height);
  await expect(preview).toHaveCSS("text-align", "center");
  const info = preview.getByTestId("workflow-move-preview-details-toggle");
  await expect(info).toHaveText("");
  if (touch) {
    const box = await info.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    await info.tap();
  } else await info.click();
  await expect(preview.getByTestId("workflow-move-preview-details")).toBeVisible();
}
