import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import { registerSeparateQueueRows } from "../../helpers/message-queue-settings";
import { dwell } from "../../helpers/causal-waits";

/** Fragment of dnd-kit's default `onDragOver` accessibility announcement. */
const MOVED_OVER = "was moved over droppable area";

registerSeparateQueueRows(test);

async function expectTouchTarget(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  // Chromium can report a 44px CSS target a fraction below 44 after layout.
  expect(box!.width).toBeGreaterThanOrEqual(43.5);
  expect(box!.height).toBeGreaterThanOrEqual(43.5);
}

async function seedBusyQueueTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile queue reorder",
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
  await session.sendMessageViaButton("/slow 30s");
  await session.agentStatus().waitFor({ state: "visible", timeout: 15_000 });
  await waitForComposerQueueMode(testPage);
  return session;
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

/**
 * Touch-pointer drag for dnd-kit's PointerSensor: pointerdown on the handle,
 * two pointermoves (the first only activates the drag, the second dispatches
 * DragMove so collision detection resolves), then pointerup. isPrimary is set
 * explicitly because Playwright's dispatchEvent constructs untrusted events.
 */
async function touchDragTo(
  page: Page,
  source: Locator,
  targetBox: { x: number; y: number },
): Promise<void> {
  const sourceBox = (await source.boundingBox())!;
  const start = { x: sourceBox.x + sourceBox.width / 2, y: sourceBox.y + sourceBox.height / 2 };
  const end = { x: targetBox.x, y: targetBox.y };
  const pointerId = 5;
  const down = {
    pointerId,
    pointerType: "touch",
    isPrimary: true,
    button: 0,
    clientX: start.x,
    clientY: start.y,
  };
  // dnd-kit announces each resolved drag target in its accessibility live
  // region; that announcement is the observable signal for the drag-start and
  // target-resolution steps between these dispatches.
  const announcedTarget = async () =>
    (await page.locator('[id^="DndLiveRegion"]').allTextContents()).find((text) =>
      text.includes(MOVED_OVER),
    ) ?? "";

  await source.dispatchEvent("pointerdown", down);
  await dwell(
    page,
    100,
    "library-timer",
    "dnd-kit's PointerSensor arms on pointerdown and only activates once a later move clears its distance constraint; nothing renders in between, so this just lets React commit the pointerdown before the activating move lands in the same task",
  );
  await page.dispatchEvent("body", "pointermove", { ...down, clientX: end.x, clientY: end.y });

  // The first move activates the drag; measuring is complete once dnd-kit can
  // announce a droppable for it. Dispatching DragMove before that resolves no
  // target and the drop silently becomes a no-op.
  await expect.poll(announcedTarget, { timeout: 5_000 }).toContain(MOVED_OVER);
  const initialAnnouncement = await announcedTarget();

  // The first move can announce the dragged row itself because it also crosses
  // the sensor's activation distance. The next move must resolve the intended
  // row before pointerup; otherwise dnd-kit accepts a self-drop and the test
  // passes through the gesture without exercising reorder at all.
  await page.dispatchEvent("body", "pointermove", {
    ...down,
    clientX: end.x,
    clientY: end.y + 2,
  });
  await expect.poll(announcedTarget, { timeout: 5_000 }).not.toBe(initialAnnouncement);

  await page.dispatchEvent("body", "pointerup", {
    ...down,
    clientX: end.x,
    clientY: end.y + 2,
  });
}

test.describe.configure({ retries: 1 });

test("mobile touch drag reorders queued messages with an always-visible handle", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const session = await seedBusyQueueTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const editor = chat.locator(".tiptap.ProseMirror:visible").first();
  const submit = testPage.getByTestId("submit-message-button");
  for (const message of ["reorder first", "reorder second", "reorder third"]) {
    await typeWhileBusy(testPage, editor, message);
    await expect(submit).toBeVisible({ timeout: 5_000 });
    await submit.tap();
  }

  const chip = chat.getByTestId("queue-chip");
  await expect(chip).toBeVisible({ timeout: 10_000 });
  await chip.tap();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible({ timeout: 5_000 });
  await expectRelativeOrder(panel, "reorder first", "reorder second", "reorder third");

  // No hover on touch: the handle is always visible and touch-sized.
  const firstHandle = rowHandle(panel, "reorder first");
  const lastHandle = rowHandle(panel, "reorder third");
  await expect(firstHandle).toHaveCSS("opacity", "1");
  await expectTouchTarget(lastHandle);

  const firstBox = (await firstHandle.boundingBox())!;
  await touchDragTo(testPage, lastHandle, {
    x: firstBox.x + firstBox.width / 2,
    y: firstBox.y + firstBox.height / 2,
  });

  await expectRelativeOrder(panel, "reorder third", "reorder first", "reorder second");

  // The reorder is persisted server-side and visible after a reload.
  await testPage.reload();
  await session.waitForLoad();
  await expect(chat.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  await chat.getByTestId("queue-chip").tap();
  const panelAfter = chat.getByTestId("queued-ghost-list");
  await expect(panelAfter).toBeVisible({ timeout: 5_000 });
  await expectRelativeOrder(panelAfter, "reorder third", "reorder first", "reorder second");
});
