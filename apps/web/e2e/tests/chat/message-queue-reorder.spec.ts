import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

/** Create a task whose agent finishes its intro turn and becomes idle. */
async function seedIdleTask(
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
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 60_000 });
  return session;
}

/** Keep the agent busy with a slow command, then queue one message per entry. */
async function queueMessagesWhileBusy(
  page: Page,
  editor: Locator,
  messages: string[],
): Promise<void> {
  const submit = page.getByTestId("submit-message-button");
  for (const message of messages) {
    await typeWhileBusy(page, editor, message);
    await expect(submit).toBeVisible({ timeout: 5_000 });
    await submit.click();
  }
}

async function openQueuePanel(session: SessionPage): Promise<Locator> {
  const chip = session.activeChat().getByTestId("queue-chip");
  await expect(chip).toBeVisible({ timeout: 10_000 });
  await chip.click();
  const panel = session.activeChat().getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible({ timeout: 5_000 });
  return panel;
}

/** Grab handle of the row whose visible text contains `text`. */
function rowHandle(panel: Locator, text: string): Locator {
  const row = panel.getByTestId("queue-entry").filter({ hasText: text });
  return row.locator("xpath=..").getByTestId("queue-grab-handle");
}

/**
 * Assert that `contents` occupy exactly the consecutive positions starting at
 * the head after removing the `/slow` keep-busy command, and that the `/slow`
 * row — when present — sits at the head. This pins the global order instead of
 * normalizing it away: a foreign row interleaved between the expected contents
 * or a `/slow` row anywhere but the head fails.
 */
async function expectRelativeOrder(panel: Locator, ...contents: string[]): Promise<void> {
  await expect
    .poll(
      async () => {
        const texts = (await panel.getByTestId("queue-entry-text").allTextContents()).map((t) =>
          t.replace(/\s+/g, " ").trim(),
        );
        const slowIndex = texts.findIndex((t) => t.startsWith("/slow"));
        const withoutSlow = texts.filter((_, index) => index !== slowIndex);
        return {
          positions: contents.map((content) => withoutSlow.findIndex((t) => t.includes(content))),
          slowHead: slowIndex === -1 || slowIndex === 0,
        };
      },
      { timeout: 10_000 },
    )
    .toEqual({ positions: contents.map((_, index) => index), slowHead: true });
}

test.describe("Queue reorder", () => {
  test.describe.configure({ retries: 1 });

  test("drag-and-drop reorders queued messages and persists the order", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedIdleTask(testPage, apiClient, seedData, "Queue reorder drag");
    await session.sendMessage("/slow 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
    await queueMessagesWhileBusy(testPage, editor, [
      "reorder first",
      "reorder second",
      "reorder third",
    ]);

    const panel = await openQueuePanel(session);
    await expectRelativeOrder(panel, "reorder first", "reorder second", "reorder third");

    // Multiple queued rows expose their dotted grab handles immediately on a
    // fine pointer, so the drag affordance is discoverable without hover.
    const firstHandle = rowHandle(panel, "reorder first");
    await expect(firstHandle).toHaveCSS("opacity", "1", { timeout: 5_000 });
    const firstContent = panel.getByTestId("queue-entry").filter({ hasText: "reorder first" });
    const [handleBox, contentBox] = await Promise.all([
      firstHandle.boundingBox(),
      firstContent.boundingBox(),
    ]);
    expect(handleBox).not.toBeNull();
    expect(contentBox).not.toBeNull();
    expect(handleBox!.width).toBeLessThanOrEqual(12);
    expect(contentBox!.x).toBeGreaterThanOrEqual(handleBox!.x + handleBox!.width + 4);

    // Drag the third row's handle onto the first row. Playwright's dragTo
    // fires a single mousemove, which only activates dnd-kit's PointerSensor
    // (distance constraint) without dispatching DragMove — the drop would
    // resolve no target. Stepped moves keep the sensor's move events flowing.
    const sourceBox = (await rowHandle(panel, "reorder third").boundingBox())!;
    const targetBox = (await firstHandle.boundingBox())!;
    await testPage.mouse.move(
      sourceBox.x + sourceBox.width / 2,
      sourceBox.y + sourceBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      targetBox.x + targetBox.width / 2,
      targetBox.y + targetBox.height / 2,
      { steps: 12 },
    );
    await testPage.mouse.up();
    await expectRelativeOrder(panel, "reorder third", "reorder first", "reorder second");

    // The order is persisted server-side and survives a reload.
    await testPage.reload();
    await session.waitForLoad();
    const panelAfter = await openQueuePanel(session);
    await expectRelativeOrder(panelAfter, "reorder third", "reorder first", "reorder second");
  });

  test("keyboard reordering moves a queued message one position", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedIdleTask(testPage, apiClient, seedData, "Queue reorder keyboard");
    await session.sendMessage("/slow 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
    await queueMessagesWhileBusy(testPage, editor, [
      "reorder first",
      "reorder second",
      "reorder third",
    ]);

    const panel = await openQueuePanel(session);
    await expectRelativeOrder(panel, "reorder first", "reorder second");

    // dnd-kit's keyboard sensor: Space starts the drag, arrows move, Space
    // drops. Wait for the drag to actually start (the row dims) before moving.
    const handle = rowHandle(panel, "reorder first");
    const row = handle.locator("xpath=..");
    await handle.focus();
    await testPage.keyboard.press("Space");
    await expect(row).toHaveClass(/opacity-40/, { timeout: 5_000 });
    // Droppable measuring happens asynchronously after the drag starts; an
    // arrow press before it completes computes no target. Settle briefly.
    await testPage.waitForTimeout(200);
    await testPage.keyboard.press("ArrowDown");
    await testPage.waitForTimeout(100);
    await testPage.keyboard.press("Space");
    await expect(row).not.toHaveClass(/opacity-40/, { timeout: 5_000 });

    await expectRelativeOrder(panel, "reorder second", "reorder first");
  });
});
