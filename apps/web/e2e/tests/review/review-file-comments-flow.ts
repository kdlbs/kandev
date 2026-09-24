import { expect, type Page, type Locator } from "@playwright/test";
import { DIFF_FILE } from "./review-fix-comments-popover-flow";

export async function openFileComment(page: Page, dialog: Locator, mobile: boolean) {
  const header = dialog.locator(
    `[data-testid="review-file-header"][data-file-path="${DIFF_FILE}"]`,
  );
  if (mobile) {
    await header.getByRole("button", { name: `More actions for ${DIFF_FILE}` }).tap();
    await page
      .getByTestId("review-file-actions-menu")
      .getByRole("menuitem", { name: "Comment on file" })
      .click();
  } else {
    await header.getByRole("button", { name: "Comment on file", exact: true }).click();
  }
  const region = dialog.getByTestId("review-file-comments");
  await expect(region.getByRole("textbox")).toBeFocused();
  return region;
}

export async function exerciseFileComment(page: Page, dialog: Locator, mobile: boolean) {
  const header = dialog.locator(
    `[data-testid="review-file-header"][data-file-path="${DIFF_FILE}"]`,
  );
  await header.getByRole("button", { name: `Collapse ${DIFF_FILE}`, exact: true }).click();
  let region = await openFileComment(page, dialog, mobile);
  await expect(region.getByRole("button", { name: /Add/ })).toBeDisabled();
  await region.getByRole("textbox").fill("Cancel this draft");
  await region.getByRole("textbox").press("Escape");
  await expect(dialog).toBeVisible();
  await expect(region).toHaveCount(0);
  await expect(header.locator("[data-review-comment-opener]")).toBeFocused();
  region = await openFileComment(page, dialog, mobile);
  await region.getByRole("textbox").fill("Whole-file feedback for the agent");
  const add = region.getByRole("button", { name: /Add/ });
  if (mobile) expect((await add.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await add.click();
  const card = region.getByTestId("review-file-comment-card");
  await expect(card).toContainText("Whole-file feedback for the agent");
  await card.getByRole("button", { name: "Edit comment", exact: true }).click();
  await card.getByRole("textbox").fill("Updated whole-file feedback");
  await card.getByRole("button", { name: "Update", exact: true }).click();
  await expect(card).toContainText("Updated whole-file feedback");
  await expect(dialog.getByTestId("review-fix-comments-button")).toContainText("1");
  await expect(page.locator("html")).toHaveJSProperty(
    "scrollWidth",
    await page.locator("html").evaluate((el) => el.clientWidth),
  );
}
