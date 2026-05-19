import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Helper to create a regular task, navigate to it, and wait for idle.
 * Mirrors clarification.spec.ts's `seedClarificationTask`, but uses a non-
 * blocking scenario so we reach the idle input and can drive the slash
 * commands `/ask-single` and `/ask-multiple` from inside the session.
 */
async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();

  return session;
}

test.describe("Mock agent clarification slash commands", () => {
  test.describe.configure({ retries: 1 });

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
    // Tall viewport so the content-sized overlay has plenty of room to grow
    // without bumping into the 50vh safety cap on the first drag.
    await testPage.setViewportSize({ width: 1280, height: 1000 });

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

    const initial = await container.evaluate((el) => {
      const computed = window.getComputedStyle(el);
      return {
        overflowY: computed.overflowY,
        height: el.getBoundingClientRect().height,
        // Inline style is what disambiguates "auto-sized" from "user-dragged".
        inlineHeight: (el as HTMLElement).style.height,
      };
    });

    expect(initial.overflowY).toBe("auto");
    // Default state: no inline height → container sizes to its content.
    expect(initial.inlineHeight).toBe("");
    // Sanity check: content-sized overlay is at least tall enough for the
    // question card but well under the 50vh cap.
    expect(initial.height).toBeGreaterThan(200);
    expect(initial.height).toBeLessThan(1000 * 0.5);

    const handle = container.locator("xpath=..").locator("button[aria-label='Resize']");
    await expect(handle).toBeVisible();

    // Drag the handle upward by 120px → overlay should grow by ~120px.
    const handleBox = await handle.boundingBox();
    expect(handleBox).not.toBeNull();
    const dragDistance = 120;
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

    // Double-click the handle → back to auto-sized.
    await handle.dblclick();
    const afterReset = await container.evaluate((el) => ({
      height: el.getBoundingClientRect().height,
      inlineHeight: (el as HTMLElement).style.height,
    }));
    expect(afterReset.inlineHeight).toBe("");
    expect(Math.abs(afterReset.height - initial.height)).toBeLessThanOrEqual(2);
  });
});
