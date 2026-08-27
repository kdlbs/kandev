// Mobile companion to message-metadata-overflow.spec.ts. The metadata dialog
// is a centered modal on every viewport; on a phone the same large
// `turn_metadata` must stay reachable through the dialog's internal entries
// scroll, with the document itself never overflowing horizontally.
import { test, expect, type Locator, type Page } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SEEDED_MESSAGE, largeTurnMetadata } from "../../helpers/message-metadata-fixtures";
import { SessionPage } from "../../pages/session-page";

/** Seeds the overflow fixture and opens the metadata dialog on this page. */
async function openMetadataDialog(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{
  dialog: Locator;
  entries: Locator;
  turnLabel: Locator;
}> {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile Metadata Overflow", {
    description: "seeded mobile message metadata overflow fixture",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: SEEDED_MESSAGE,
    turnMetadata: largeTurnMetadata(),
  });

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();

  const chat = session.activeChat();
  await expect(chat.getByText(SEEDED_MESSAGE, { exact: true })).toBeVisible({ timeout: 15_000 });

  const messageBody = chat
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: SEEDED_MESSAGE });
  const actionsRow = messageBody.locator("xpath=..");

  await actionsRow.getByRole("button", { name: "Show message metadata" }).tap();

  const dialog = page.locator('[data-slot="dialog-content"]');
  await expect(dialog).toBeVisible();
  // Strict locator: Playwright fails the test if the selector ever resolves
  // to more than one entries container (the header is the only other direct
  // child and carries data-slot="dialog-header").
  const entries = dialog.locator("> div.grid");
  await expect(entries).toBeVisible();
  return { dialog, entries, turnLabel: dialog.getByText("turn_metadata", { exact: true }) };
}

test.describe("Chat message metadata dialog overflow (mobile)", () => {
  test("scrolls entries internally with no document horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { dialog, entries, turnLabel } = await openMetadataDialog(testPage, apiClient, seedData);

    // The entries area must be scrollable on the phone viewport too.
    const scrollable = await entries.evaluate((el) => {
      const container = el as HTMLElement;
      return { scrollHeight: container.scrollHeight, clientHeight: container.clientHeight };
    });
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    await entries.evaluate((el) => {
      (el as HTMLElement).scrollTop = (el as HTMLElement).scrollHeight;
    });

    const dialogBox = await dialog.boundingBox();
    const turnBox = await turnLabel.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(turnBox).not.toBeNull();
    expect(turnBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(turnBox!.y + turnBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    // The close control is an absolute sibling outside the entries scroller;
    // it must stay fully inside the dialog while the entries are scrolled to
    // the bottom, and it must still close the dialog.
    const close = dialog.locator('[data-slot="dialog-close"]');
    await expect(close).toBeVisible();
    const closeBox = await close.boundingBox();
    expect(closeBox).not.toBeNull();
    expect(closeBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(closeBox!.y + closeBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    // The entries scroll region must be keyboard-reachable so keyboard-only
    // users can scroll to the overflowed turn_metadata field.
    await entries.focus();
    await expect(entries).toBeFocused();

    await close.tap();
    await expect(dialog).toBeHidden();

    // The dialog's internal scroll must never spill into document overflow.
    const documentOverflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(documentOverflow.scrollWidth).toBeLessThanOrEqual(documentOverflow.clientWidth);
  });
});
