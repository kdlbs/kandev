import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { watchWs } from "../../helpers/causal-waits";
import {
  openTimestampQueue,
  queuePayload,
  seedTimestampQueue,
  timestampEntries,
} from "./queued-message-timestamps.helpers";

test.use({ locale: "en-US", timezoneId: "UTC" });

// @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1, AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2
test("queue timestamps retain the newest source through reordered repeated merges and reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { task, identity } = await seedTimestampQueue(apiClient, seedData);
  for (const text of [
    "First timestamp message",
    "Second timestamp message",
    "Newest timestamp message",
  ]) {
    await apiClient.queueMessage(identity, text);
  }
  const entries = await timestampEntries(apiClient, identity);
  expect(entries).toHaveLength(3);
  const newest = entries[2];
  expect(newest.queued_at).not.toBe(entries[0].queued_at);
  await apiClient.wsRequest("message.queue.reorder", {
    ...queuePayload(identity),
    ordered_ids: [newest.id, entries[0].id, entries[1].id],
  });
  const gateway = watchWs(testPage);
  const panel = await openTimestampQueue(testPage, task.id);
  const rows = panel.getByTestId("queue-entry");
  await expect(rows.first().locator("time")).toHaveAttribute("datetime", newest.queued_at);
  for (const count of [2, 1]) {
    const merged = gateway.waitForResponse("message.queue.merge");
    await rows.nth(1).getByTestId("queue-entry-merge").click();
    await merged;
    await expect.poll(async () => (await timestampEntries(apiClient, identity)).length).toBe(count);
    await expect(rows).toHaveCount(count);
    await expect(rows.first().locator("time")).toHaveAttribute("datetime", newest.queued_at);
  }
  const reloaded = await openTimestampQueue(testPage, task.id);
  await expect(reloaded.getByTestId("queue-entry").locator("time")).toHaveAttribute(
    "datetime",
    newest.queued_at,
  );
  await waitForFiniteAnimations(testPage.locator("body"));
  await prCapture.screenshot("queued-message-timestamp-desktop", {
    caption: "Merged queue row retains the newest contributing timestamp.",
  });
});

test("automatic merging updates a visible queue timestamp and survives reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { task, identity } = await seedTimestampQueue(apiClient, seedData);
  await apiClient.setQueueAutoMerge(identity, true);
  await apiClient.queueMessage(identity, "Automatic first message");
  const first = (await timestampEntries(apiClient, identity))[0];
  const panel = await openTimestampQueue(testPage, task.id);
  await expect(panel.locator("time")).toHaveAttribute("datetime", first.queued_at);
  await apiClient.queueMessage(identity, "Automatic newest message");
  const merged = await timestampEntries(apiClient, identity);
  expect(merged).toHaveLength(1);
  expect(merged[0].id).toBe(first.id);
  expect(merged[0].queued_at).not.toBe(first.queued_at);
  await expect(panel.getByTestId("queue-entry-text")).toContainText("Automatic newest message");
  await expect(panel.locator("time")).toHaveAttribute("datetime", merged[0].queued_at);
  const reloaded = await openTimestampQueue(testPage, task.id);
  await expect(reloaded.locator("time")).toHaveAttribute("datetime", merged[0].queued_at);
});

test("Quick Chat queue displays its server timestamp", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const chat = await apiClient.startQuickChat(
    seedData.workspaceId,
    seedData.agentProfileId,
    "Timestamp Quick Chat",
  );
  const identity = await apiClient.getQueueSessionIdentity(chat.task_id, chat.session_id);
  await apiClient.setQueueAutoRun(identity, false);
  await apiClient.queueMessage(identity, "Quick Chat timestamp message");
  const queued = (await timestampEntries(apiClient, identity))[0];
  await testPage.goto("/");
  await testPage.getByTestId("sidebar-quick-chat-shortcut").click();
  const dialog = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
  await dialog.getByTestId("queue-chip").click();
  await expect(
    dialog
      .getByTestId("queue-entry")
      .filter({ hasText: "Quick Chat timestamp message" })
      .locator("time"),
  ).toHaveAttribute("datetime", queued.queued_at);
});
