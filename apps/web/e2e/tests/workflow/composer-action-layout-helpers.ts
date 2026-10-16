import type { Locator } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

export const COMPOSER_HISTORY_PROMPT = Array.from(
  { length: 30 },
  (_, index) =>
    `e2e:message(${JSON.stringify(`Transcript entry ${index + 1}: ${"Review the implementation and record the result. ".repeat(4)}`)})`,
).join("\n");

/** Keeps the transcript near its end while exposing every navigation control. */
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

/** Checks keyboard progression between the primary action and visible utilities. */
export async function expectComposerActionKeyboardOrder(chat: Locator): Promise<void> {
  const row = chat.getByTestId("chat-status-bar");
  const proceed = row.getByTestId("proceed-next-step");
  const controls = row.getByTestId("chat-status-bar-right-controls").locator("button:visible");
  const phone = (await chat.page().evaluate(() => window.innerWidth)) < 768;
  if (phone) {
    await proceed.focus();
    await chat.page().keyboard.press("Tab");
    await expect(controls.first()).toBeFocused();
  } else {
    await controls.last().focus();
    await chat.page().keyboard.press("Tab");
    await expect(proceed).toBeFocused();
    await chat.page().keyboard.press("Tab");
  }
}

/** Removes transient hover/focus chrome before capturing the composer layout. */
export async function prepareComposerCapture(chat: Locator): Promise<void> {
  const page = chat.page();
  await page.mouse.move(0, 0);
  await page.evaluate(() => {
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  });
  await expect(page.locator('[data-slot="tooltip-content"]:visible')).toHaveCount(0);
  await expect(chat.getByTestId("proceed-next-step-options")).toBeHidden();
}

/**
 * Checks rendered line placement, containment, touch targets, and page overflow.
 * @covers AC-UI-COMPOSER-ACTION-WRAP-001.1, .2, .3, .5
 */
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
  const controlBoxes = await controls.locator("button:visible").evaluateAll((elements) =>
    elements.map((element) => {
      const { x, y, width, height } = element.getBoundingClientRect();
      return { x, y, width, height };
    }),
  );
  expect(controlBoxes.length).toBeGreaterThan(0);
  const controlsBox = {
    x: Math.min(...controlBoxes.map((box) => box.x)),
    y: Math.min(...controlBoxes.map((box) => box.y)),
    right: Math.max(...controlBoxes.map((box) => box.x + box.width)),
    bottom: Math.max(...controlBoxes.map((box) => box.y + box.height)),
  };
  const proceedBox = await proceed.boundingBox();
  expect(rowBox).not.toBeNull();
  expect(proceedBox).not.toBeNull();
  if (!rowBox || !proceedBox) throw new Error("composer controls are not measurable");

  const phone = (await chat.page().evaluate(() => window.innerWidth)) < 768;
  if (phone) {
    expect(proceedBox.y + proceedBox.height).toBeLessThanOrEqual(controlsBox.y);
  } else if (wrapped) {
    expect(proceedBox.y).toBeGreaterThanOrEqual(controlsBox.bottom);
  } else if (wrapped === false) {
    expect(
      Math.abs(proceedBox.y + proceedBox.height / 2 - (controlsBox.y + controlsBox.bottom) / 2),
    ).toBeLessThanOrEqual(1);
    expect(proceedBox.x).toBeGreaterThanOrEqual(controlsBox.right);
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
  } else {
    expect(proceedBox.height).toBeCloseTo(24, 0);
  }
  if (await proceed.isEnabled()) {
    await expect
      .poll(() =>
        proceed.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
          );
        }),
      )
      .toBe(true);
  }
  await expect(actions).toHaveCSS("display", "flex");
  if (!phone) await expect(actions).toHaveCSS("justify-content", "flex-end");
  await assertNoDocumentHorizontalOverflow(chat.page());
}
