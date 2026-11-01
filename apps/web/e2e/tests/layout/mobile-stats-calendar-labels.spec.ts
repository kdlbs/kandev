import { test } from "../../fixtures/test-base";
import { assertCalendarBuckets, openCalendarStats } from "./stats-calendar-labels-helpers";

test.use({ locale: "en-US", timezoneId: "America/Los_Angeles" });

// @covers AC-UI-STATS-CALENDAR-LABELS-001.1, .2, .4, .5
test("phone bucket taps preserve calendar attribution", async ({
  page,
  testPage,
  backend,
  seedData,
}, testInfo) => {
  void testPage;
  const card = await openCalendarStats(page, backend, seedData.workspaceId, "America/Los_Angeles");
  await assertCalendarBuckets(page, card, true);
  await card.screenshot({ path: testInfo.outputPath("phone-calendar-chart.png") });
});
