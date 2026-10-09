import type { Locator } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

export const COMPOSER_HISTORY_PROMPT = Array.from(
  { length: 30 },
  (_, index) =>
    `e2e:message(${JSON.stringify(`Transcript entry ${index + 1}: ${"Review the implementation and record the result. ".repeat(4)}`)})`,
).join("\n");

export async function revealTranscriptControls(chat: Locator): Promise<void> {
  const list = chat.locator(".chat-message-list");
  await expect
    .poll(() => list.evaluate((element) => element.scrollHeight - element.clientHeight))
    .toBeGreaterThan(200);
  await list.evaluate((element) => {
    element.scrollTop = Math.max(0, element.scrollHeight - element.clientHeight - 80);
  });
  await expect(chat.getByTestId("jump-to-latest-button")).toBeVisible();
  await expect(chat.getByTestId("auto-scroll-toggle-button")).toBeVisible();
  await expect(chat.getByTestId("scroll-to-start-button")).toBeVisible();
  await expect(chat.getByTestId("scroll-to-last-prompt-button")).toBeVisible();
}

/** @covers AC-UI-COMPOSER-ACTION-WRAP-001.1, .2, .3, .5 */
export async function expectComposerActionLayout(
  chat: Locator,
  { wrapped, touch }: { wrapped?: boolean; touch: boolean },
): Promise<void> {
  const row = chat.getByTestId("chat-status-bar");
  const actions = row.getByTestId("chat-status-bar-actions");
  const controls = row.getByTestId("chat-status-bar-right-controls");
  const proceed = row.getByTestId("proceed-next-step");
  await expect(proceed).toHaveCount(1);
  await expect(proceed).toBeVisible();
  const rowBox = await row.boundingBox();
  const controlsBox = await controls.boundingBox();
  const proceedBox = await proceed.boundingBox();
  expect(rowBox).not.toBeNull();
  expect(controlsBox).not.toBeNull();
  expect(proceedBox).not.toBeNull();
  if (!rowBox || !controlsBox || !proceedBox)
    throw new Error("composer controls are not measurable");

  if (wrapped) {
    expect(proceedBox.y).toBeGreaterThanOrEqual(controlsBox.y + controlsBox.height);
  } else if (wrapped === false) {
    expect(
      Math.abs(proceedBox.y + proceedBox.height / 2 - controlsBox.y - controlsBox.height / 2),
    ).toBeLessThanOrEqual(1);
    expect(proceedBox.x).toBeGreaterThanOrEqual(controlsBox.x + controlsBox.width);
  }
  const padding = await row.evaluate((element) => {
    const style = getComputedStyle(element);
    return { left: parseFloat(style.paddingLeft), right: parseFloat(style.paddingRight) };
  });
  expect(
    Math.abs(proceedBox.x + proceedBox.width - (rowBox.x + rowBox.width - padding.right)),
  ).toBeLessThanOrEqual(1);
  expect(proceedBox.x).toBeGreaterThanOrEqual(rowBox.x + padding.left);
  expect(proceedBox.y + proceedBox.height).toBeLessThanOrEqual(rowBox.y + rowBox.height + 1);
  if (touch) {
    expect(proceedBox.width).toBeGreaterThanOrEqual(44);
    expect(proceedBox.height).toBeGreaterThanOrEqual(44);
  }
  await expect(actions).toHaveCSS("justify-content", "flex-end");
  await assertNoDocumentHorizontalOverflow(chat.page());
}
