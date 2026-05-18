import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

// These tests run under the mobile-chrome Playwright project (Pixel 5,
// viewport 393x851). They catch layout regressions that a desktop viewport
// hides: horizontal overflow and clipped-off-viewport interactive elements
// on the office onboarding wizard.

test.describe("Office onboarding — mobile layout", () => {
  test("setup wizard does not overflow horizontally on Pixel 5", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();

    const overflow = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(overflow.scroll).toBeLessThanOrEqual(overflow.client);
  });

  test("close button is fully inside the viewport on mobile", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();

    const close = testPage.getByRole("button", { name: "Cancel" });
    const box = await close.boundingBox();
    const viewport = testPage.viewportSize();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (!box || !viewport) return;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
  });

  test("agent step (Step 1) fits within the viewport horizontally", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /next/i }).click();
    await expect(
      testPage.getByRole("heading", { name: "Create your coordinator agent" }),
    ).toBeVisible();

    const overflow = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(overflow.scroll).toBeLessThanOrEqual(overflow.client);

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    if (!viewport) return;

    // Every visible interactive control on this step must fit inside the
    // viewport horizontally. Catches comboboxes forcing a min-width wider
    // than the phone screen, or a row whose icon + label still exceeds it.
    const inputs = testPage.locator("input, button, [role=combobox]");
    const count = await inputs.count();
    for (let i = 0; i < count; i++) {
      const el = inputs.nth(i);
      const box = await el.boundingBox();
      if (!box) continue; // hidden
      const tag = await describeElement(el);
      expect(box.x + box.width, tag).toBeLessThanOrEqual(viewport.width + 1);
      expect(box.x, tag).toBeGreaterThanOrEqual(-1);
    }
  });

  test("agent step (Step 1) heading and Next button are reachable on mobile", async ({
    testPage,
    officeSeed: _,
  }) => {
    // The bug we are catching: `fixed inset-0 ... flex items-center` centers
    // content that is taller than the phone viewport, so both the step heading
    // (top of Step 1) and the Next button (bottom of Step 1) end up clipped
    // off-screen — and because the wrapper has no overflow-y-auto, the user
    // cannot scroll to reach them. The whole step becomes unusable.
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /next/i }).click();

    const heading = testPage.getByRole("heading", { name: "Create your coordinator agent" });
    await expect(heading).toBeVisible();

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    if (!viewport) return;

    // The user has to be able to reach the controls without scrolling magic.
    // We allow scrolling, but the heading must sit inside the document
    // (positive y) and the Next button must be reachable by scrolling the
    // page (its bottom must be <= scrollHeight, i.e. inside the document).
    const headingBox = await heading.boundingBox();
    expect(headingBox).not.toBeNull();
    if (!headingBox) return;
    expect(
      headingBox.y,
      "step heading should not be clipped above the document",
    ).toBeGreaterThanOrEqual(-1);

    const next = testPage.getByRole("button", { name: /next/i });
    const nextBox = await next.boundingBox();
    expect(nextBox).not.toBeNull();
    if (!nextBox) return;
    const scrollHeight = await testPage.evaluate(() => document.documentElement.scrollHeight);
    expect(
      nextBox.y + nextBox.height,
      "Next button must be reachable within the scrollable document",
    ).toBeLessThanOrEqual(scrollHeight + 1);

    // And: actually clicking Next must advance the wizard. If the wrapper
    // is unscrollable and Next is below the viewport, Playwright's auto-
    // scroll cannot bring it on-screen and the click times out — which is
    // exactly the user-facing bug.
    await next.click();
    await expect(
      testPage.getByRole("heading", { name: /Give your .* something to do/i }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("opening the agent profile combobox keeps the document within the viewport", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await testPage.getByRole("button", { name: /next/i }).click();
    await expect(
      testPage.getByRole("heading", { name: "Create your coordinator agent" }),
    ).toBeVisible();

    await testPage.getByTestId("agent-profile-selector").click();
    await testPage.waitForTimeout(150); // let the popover position

    const overflow = await testPage.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(overflow.scroll).toBeLessThanOrEqual(overflow.client);
  });
});

async function describeElement(el: Locator): Promise<string> {
  return el
    .evaluate((node: Element) => {
      const tid = node.getAttribute("data-testid");
      if (tid) return `[data-testid="${tid}"]`;
      const label = node.getAttribute("aria-label");
      const text = (node.textContent ?? "").trim().slice(0, 30);
      const id = node.id ? `#${node.id}` : "";
      return `${node.tagName.toLowerCase()}${id}${label ? `[aria-label="${label}"]` : ""}${text ? ` "${text}"` : ""}`;
    })
    .catch(() => "(unknown element)");
}
