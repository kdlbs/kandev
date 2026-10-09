import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import {
  forceIdleConversationGap,
  waitForRecoveredMessagesApplied,
  watchLoadingRows,
} from "../../helpers/turn-end-history-refresh";

const USAGE_PROMPT = "Reply briefly /with-usage";

test("mobile idle recovery preserves transcript scroll without a loading row", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const gap = await forceIdleConversationGap(testPage);
  gap.holdRecoveryResponse = true;
  const session = await seedIdleSession(
    testPage,
    apiClient,
    seedData,
    "Mobile silent gap recovery",
  );
  const loadingRowSeen = await watchLoadingRows(testPage);

  gap.armed = true;
  await session.sendMessageViaButton(USAGE_PROMPT);
  await expect.poll(() => gap.completed, { timeout: 30_000 }).toBe(true);
  await expect.poll(() => gap.recoveryResponseReceived, { timeout: 20_000 }).toBe(true);

  const transcript = session.activeChat().locator(".chat-message-list");
  const scrollTopBeforeApply = await transcript.evaluate((element) => element.scrollTop);
  gap.releaseRecoveryResponse();
  await expect.poll(() => gap.recovered, { timeout: 5_000 }).toBe(true);
  await waitForRecoveredMessagesApplied(testPage, gap);

  await expect(session.activeChat().getByText(USAGE_PROMPT, { exact: true })).toBeVisible();
  expect(await loadingRowSeen()).toBe(false);
  expect(await transcript.evaluate((element) => element.scrollTop)).toBe(scrollTopBeforeApply);
});
