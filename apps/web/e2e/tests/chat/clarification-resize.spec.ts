import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { seedClarificationSession } from "../../helpers/clarification";
import { SessionPage } from "../../pages/session-page";

/**
 * Create a regular task, navigate to it, and wait for idle. Uses a non-blocking
 * scenario so we reach the idle input and can drive the slash commands
 * `/ask-single` and `/ask-multiple` from inside the session.
 */
function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  return seedClarificationSession(testPage, apiClient, seedData, title, {
    scenario: "simple-message",
    waitForIdle: true,
  });
}

test.describe("Mock agent clarification slash commands", () => {
  // Smoke test that the /ask-single alias routes to the clarification scenario.
  // The underlying clarification behaviour (option click, multi-question carousel,
  // submit, etc.) is exhaustively covered by clarification.spec.ts — this only
  // proves the alias is wired up.
  test("/ask-single alias triggers the clarification overlay", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskAndWaitForIdle(testPage, apiClient, seedData, "Ask Single Alias");

    await session.sendMessage("/ask-single");

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationOption("PostgreSQL").click();
    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("/ask-multiple alias triggers the multi-question carousel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Ask Multiple Alias",
    );

    await session.sendMessage("/ask-multiple");

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await expect(session.clarificationSteps()).toHaveCount(3);
    await expect(session.clarificationOverlay()).toContainText("Which database");

    await session.clarificationOption("PostgreSQL").click();
    await session.clarificationOption("Go").click();
    await session.clarificationOption("Docker").click();

    await session.clarificationSubmit().click();
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });
});

test.describe("Clarification overlay resizable layout", () => {
  test("starts content-sized, drag grows the overlay, double-click resets", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Tall viewport so the content-sized overlay has room to grow without
    // reaching the initial 50vh cap on the first drag.
    await testPage.setViewportSize({ width: 1280, height: 3000 });

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Clarification Resize",
    );

    await session.sendMessage("/ask-multiple");
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const container = testPage.getByTestId("clarification-overlay-container");
    await expect(container).toBeVisible();

    // The container also carries the persistent header row (size toggle,
    // collapse toggle + waiting count), so it does not scroll itself — the
    // nested scroll region below the header does.
    const scrollRegion = container.getByTestId("clarification-scroll-region");
    const initial = await container.evaluate((el) => {
      return {
        height: el.getBoundingClientRect().height,
        // Inline style is what disambiguates "auto-sized" from "user-dragged".
        inlineHeight: (el as HTMLElement).style.height,
      };
    });
    await expect(scrollRegion).toHaveCSS("overflow-y", "auto");
    // Default state: no inline height → container sizes to its content.
    expect(initial.inlineHeight).toBe("");
    // Sanity check: content-sized overlay is at least tall enough for the
    // question card but well under the 50vh safety cap.
    expect(initial.height).toBeGreaterThan(200);
    expect(initial.height).toBeLessThan(3000 * 0.5);

    const toggle = testPage.getByTestId("clarification-panel-size-toggle");
    await expect(toggle).toHaveCount(1);
    await expect(toggle).toHaveAttribute("aria-label", "Expand");
    await expect(toggle).toBeEnabled();

    // The tooltip explains the same action as the accessible name.
    await testPage.mouse.move(0, 0);
    await toggle.hover();
    await expect(testPage.getByRole("tooltip").first()).toContainText("Expand");
    await testPage.mouse.move(0, 0);

    // Keyboard activation expands to the measured host and keeps focus on the
    // same control while its next action becomes Restore.
    await toggle.focus();
    await testPage.keyboard.press("Enter");
    const hostMax = Number(await container.getAttribute("data-host-max-height"));
    await expect(container).toHaveCSS("height", `${hostMax}px`);
    await expect(toggle).toHaveAttribute("aria-label", "Restore");
    await expect(toggle).toBeFocused();

    await toggle.focus();
    await testPage.keyboard.press("Space");
    await expect.poll(() => container.evaluate((el) => (el as HTMLElement).style.height)).toBe("");
    await expect(toggle).toHaveAttribute("aria-label", "Expand");

    const handle = container.locator("xpath=..").locator("button[aria-label='Resize']");
    await expect(handle).toBeVisible();

    // Drag the handle upward by 40px; the overlay should grow by roughly the same amount.
    const handleBox = await handle.boundingBox();
    expect(handleBox).not.toBeNull();
    const dragDistance = 40;
    const startX = handleBox!.x + handleBox!.width / 2;
    const startY = handleBox!.y + handleBox!.height / 2;
    await testPage.mouse.move(startX, startY);
    await testPage.mouse.down();
    await testPage.mouse.move(startX, startY - dragDistance, { steps: 10 });
    await testPage.mouse.up();

    const afterDrag = await container.evaluate((el) => ({
      height: el.getBoundingClientRect().height,
      inlineHeight: (el as HTMLElement).style.height,
    }));
    // Drag flipped the container to an explicit pixel height.
    expect(afterDrag.inlineHeight).toMatch(/^\d+(\.\d+)?px$/);
    // Allow ±10px tolerance for rounding / mouse event coalescing.
    expect(afterDrag.height).toBeGreaterThan(initial.height + dragDistance - 10);

    expect(hostMax).toBeGreaterThan(afterDrag.height);
    await expect(toggle).toBeEnabled();
    await toggle.click();
    await expect(container).toHaveCSS("height", `${hostMax}px`);
    await expect(toggle).toHaveAttribute("aria-label", "Restore");

    // Restore returns to the manual height the user selected, not the cap.
    await toggle.click();
    await expect
      .poll(() => container.evaluate((el) => (el as HTMLElement).style.height))
      .toBe(afterDrag.inlineHeight);
    await expect(toggle).toHaveAttribute("aria-label", "Expand");

    // Double-click the handle → back to auto-sized.
    await handle.dblclick();
    const afterReset = await container.evaluate((el) => ({
      height: el.getBoundingClientRect().height,
      inlineHeight: (el as HTMLElement).style.height,
    }));
    expect(afterReset.inlineHeight).toBe("");
    expect(Math.abs(afterReset.height - initial.height)).toBeLessThanOrEqual(2);
  });

  test("follows host contraction while expanded and keeps Restore usable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 1100 });

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Clarification Resize Host Changes",
    );

    await session.sendMessage("/e2e:clarification-markdown");
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const container = session.clarificationBar();
    const toggle = container.getByTestId("clarification-panel-size-toggle");
    await expect(toggle).toHaveAttribute("aria-label", "Expand");
    await expect(toggle).toBeEnabled();
    await toggle.click();
    const hostBefore = Number(await container.getAttribute("data-host-max-height"));
    await expect(container).toHaveCSS("height", `${hostBefore}px`);

    await testPage.setViewportSize({ width: 1280, height: 700 });
    await expect
      .poll(async () => Number(await container.getAttribute("data-host-max-height")))
      .toBeLessThan(hostBefore);
    await expect
      .poll(() =>
        container.evaluate((element) => {
          const host = Number((element as HTMLElement).dataset.hostMaxHeight);
          return Math.abs(element.getBoundingClientRect().height - host) <= 1;
        }),
      )
      .toBe(true);
    await expect(toggle).toHaveAttribute("aria-label", "Restore");
    await expect(toggle).toBeEnabled();

    await toggle.focus();
    await testPage.keyboard.press("Enter");
    await expect.poll(() => container.evaluate((el) => (el as HTMLElement).style.height)).toBe("");
    await expect(toggle).toHaveAttribute("aria-label", "Expand");
    await expect(toggle).toBeFocused();
  });

  test("keeps Expand visible but disabled when the host has no spare space", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 420 });

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Clarification Resize No Headroom",
    );

    await session.sendMessage("/e2e:clarification-markdown");
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const container = session.clarificationBar();
    const toggle = container.getByTestId("clarification-panel-size-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toHaveAttribute("aria-label", "Expand");
    await expect(toggle).toBeDisabled();

    const geometry = await container.evaluate((element) => ({
      hostMax: Number((element as HTMLElement).dataset.hostMaxHeight),
      height: element.getBoundingClientRect().height,
      inlineHeight: (element as HTMLElement).style.height,
    }));
    expect(geometry.inlineHeight).toBe("");
    expect(geometry.hostMax).toBeGreaterThan(0);
    expect(geometry.height).toBeGreaterThanOrEqual(geometry.hostMax - 1);
  });

  // The initial 50vh cap remains the default. Manual resizing can use only
  // the measured task-chat host budget, which may be smaller than the viewport.
  test("dragging past the initial cap grows the task panel to its measured host bound", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Clarification Resize Host Bound",
    );

    await session.sendMessage("/e2e:clarification-markdown");
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const container = session.clarificationBar();
    await expect(container).toBeVisible();
    const geometry = await container.evaluate((el) => ({
      hostMax: Number((el as HTMLElement).dataset.hostMaxHeight),
      initialMax: Number.parseFloat((el as HTMLElement).style.maxHeight),
      viewportHeight: window.innerHeight,
      contentHeight: (
        el.querySelector('[data-testid="clarification-scroll-region"]') as HTMLElement
      ).scrollHeight,
      contentClientHeight: (
        el.querySelector('[data-testid="clarification-scroll-region"]') as HTMLElement
      ).clientHeight,
    }));
    const initialCap = Math.round(geometry.viewportHeight * 0.5);
    expect(geometry.hostMax).toBeGreaterThan(initialCap);
    expect(geometry.initialMax).toBe(initialCap);
    expect(geometry.contentHeight).toBeGreaterThan(geometry.contentClientHeight);

    const handle = container.locator("xpath=..").locator("button[aria-label='Resize']");
    await expect(handle).toBeVisible();

    // A large real mouse gesture crosses the 50vh initial cap and exercises
    // the independently measured host ceiling.
    const handleBox = await handle.boundingBox();
    expect(handleBox).not.toBeNull();
    const startX = handleBox!.x + handleBox!.width / 2;
    const startY = handleBox!.y + handleBox!.height / 2;
    await testPage.mouse.move(startX, startY);
    await testPage.mouse.down();
    await testPage.mouse.move(startX, startY - 1800, { steps: 20 });
    await testPage.mouse.up();

    const afterDrag = await container.evaluate((el) => ({
      height: el.getBoundingClientRect().height,
      inlineHeight: (el as HTMLElement).style.height,
    }));

    const hostMaxAfterDrag = Number(await container.getAttribute("data-host-max-height"));
    expect(afterDrag.inlineHeight).toBe(`${hostMaxAfterDrag}px`);
    await expect(container).toHaveCSS("max-height", `${hostMaxAfterDrag}px`);
    expect(afterDrag.height).toBeCloseTo(hostMaxAfterDrag, 0);
  });
});
