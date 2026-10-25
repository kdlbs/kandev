import { test } from "../../fixtures/test-base";
import {
  assertActivityCalendarDate,
  assertCalendarBuckets,
  openCalendarStats,
} from "./stats-calendar-labels-helpers";

const zones = [
  { name: "Los Angeles", timezoneId: "America/Los_Angeles" },
  { name: "UTC", timezoneId: "UTC" },
  { name: "Tokyo", timezoneId: "Asia/Tokyo" },
];

// @covers AC-UI-STATS-CALENDAR-LABELS-001.1 through .5
for (const zone of zones) {
  test.describe(zone.name, () => {
    test.use({ locale: "en-US", timezoneId: zone.timezoneId });

    test("calendar keys retain labels, counts and tooltip attribution", async ({
      page,
      testPage,
      backend,
      seedData,
    }, testInfo) => {
      // testPage owns the backend reset; page inherits the configured timezone.
      void testPage;
      const card = await openCalendarStats(page, backend, seedData.workspaceId, zone.timezoneId);
      await assertCalendarBuckets(page, card);
      await card.screenshot({ path: testInfo.outputPath("desktop-calendar-chart.png") });
      await assertActivityCalendarDate(page);
    });
  });
}
