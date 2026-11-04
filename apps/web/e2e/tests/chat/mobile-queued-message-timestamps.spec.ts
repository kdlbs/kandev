import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { watchWs } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  openTimestampQueue,
  seedTimestampQueue,
  timestampEntries,
} from "./queued-message-timestamps.helpers";

test.use({ locale: "en-US", timezoneId: "UTC" });

// @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1, AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2, AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3, AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
for (const width of [393, 390]) {
  test(`queue timestamps show merged time and touch details at ${width}px`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width, height: 851 });
    const originalDisplay = (await apiClient.getUserSettings()).settings.message_time_display;
    try {
      await apiClient.saveUserSettings({ message_time_display: "absolute_long" });
      const { task, identity } = await seedTimestampQueue(apiClient, seedData);
      await apiClient.queueMessage(identity, "First mobile message");
      await apiClient.queueMessage(identity, "Newest mobile message");
      const newest = (await timestampEntries(apiClient, identity))[1];
      const gateway = watchWs(testPage);
      const panel = await openTimestampQueue(testPage, task.id);
      const merged = gateway.waitForResponse("message.queue.merge");
      await panel.getByTestId("queue-entry-merge").tap();
      await merged;
      await expect(panel.getByTestId("queue-entry")).toHaveCount(1);
      const row = panel.getByTestId("queue-entry");
      const timestamp = row.locator("time");
      await expect(timestamp).toHaveAttribute("datetime", newest.queued_at);
      const label = new Intl.DateTimeFormat("en-US", {
        dateStyle: "long",
        timeStyle: "medium",
        timeZone: "UTC",
      }).format(new Date(newest.queued_at));
      await expect(timestamp).toHaveText(label);
      const trigger = row.getByTestId("message-timestamp-trigger");
      const box = await trigger.boundingBox();
      const rowBox = await row.boundingBox();
      expect(box).not.toBeNull();
      expect(rowBox).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(rowBox!.x);
      expect(box!.x + box!.width).toBeLessThanOrEqual(rowBox!.x + rowBox!.width);
      await expect(row.getByTestId("queue-entry-remove")).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage);
      await prCapture.screenshot(`queued-message-timestamp-mobile-${width}`, {
        caption: "Long queue timestamp remains contained beside touch actions.",
      });
      const counterpart = await timestamp.getAttribute("title");
      await trigger.tap();
      const drawer = testPage.getByTestId("message-timestamp-drawer");
      await expect(drawer).toBeVisible();
      await expect(drawer).toContainText(counterpart!);
      await waitForFiniteAnimations(drawer);
      await expect(drawer.getByText(counterpart!, { exact: true })).toBeInViewport();
      await prCapture.screenshot(`queued-message-timestamp-details-${width}`, {
        caption: "Tapping the queued timestamp reveals its counterpart.",
      });
      await testPage.keyboard.press("Escape");
      await expect(drawer).toBeHidden();
      await expect(row.getByTestId("queue-entry-remove")).toBeEnabled();
    } finally {
      await apiClient.saveUserSettings({ message_time_display: originalDisplay ?? "relative" });
    }
  });
}
