import { expect, type Locator } from "@playwright/test";
import { waitForFiniteAnimations } from "./animations";
import { requireBox } from "./layout-assertions";

/** Short decisions use content height, not an invisible editor's remaining space. */
export async function expectContentSizedBottomConfirmation(
  surface: Locator,
  confirmation: Locator,
) {
  await waitForFiniteAnimations(surface);
  expect(await surface.evaluate((element) => element.scrollTop)).toBe(0);
  const surfaceBox = await requireBox(surface, "confirmation sheet");
  const viewportHeight = surface.page().viewportSize()!.height;
  expect(surfaceBox.y + surfaceBox.height).toBeCloseTo(viewportHeight, 0);
  expect(surfaceBox.y).toBeGreaterThanOrEqual(0);
  const contentBox = await requireBox(
    confirmation.getByTestId("mobile-confirmation-body").locator(":scope > :last-child"),
    "last consequence or option",
  );
  const primaryBox = await requireBox(confirmation.locator("footer button").first(), "action");
  const actionGap = primaryBox.y - contentBox.y - contentBox.height;
  expect(actionGap).toBeGreaterThanOrEqual(0);
  expect(actionGap).toBeLessThanOrEqual(32);
  for (const control of await confirmation.locator("h2, footer button").all()) {
    await expect(control).toBeInViewport({ ratio: 1 });
  }
  return surfaceBox;
}
