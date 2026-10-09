import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import {
  forceIdleConversationGap,
  waitForRecoveredMessagesApplied,
  watchLoadingRows,
} from "../../helpers/turn-end-history-refresh";

// The mock agent reports token usage for this prompt, as real agents do every turn.
const USAGE_PROMPT = "Reply briefly /with-usage";

test.describe.configure({ retries: 0 });

test("a conversation gap found after a turn is recovered without a loading row", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const gap = await forceIdleConversationGap(testPage);
  gap.holdRecoveryResponse = true;
  const session = await seedIdleSession(testPage, apiClient, seedData, "Silent gap recovery");
  const loadingRowSeen = await watchLoadingRows(testPage);

  gap.armed = true;
  await session.sendMessage(USAGE_PROMPT);
  await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
  // Wait for the matching successful response, then release it to the app.
  await expect.poll(() => gap.recoveryResponseReceived, { timeout: 20_000 }).toBe(true);
  gap.releaseRecoveryResponse();
  await expect.poll(() => gap.recovered, { timeout: 5_000 }).toBe(true);
  await waitForRecoveredMessagesApplied(testPage, gap);
  await expect(session.activeChat().getByText(USAGE_PROMPT, { exact: true })).toBeVisible();

  expect(await loadingRowSeen()).toBe(false);
});
