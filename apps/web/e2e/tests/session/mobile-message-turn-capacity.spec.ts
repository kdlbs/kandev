// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
  assertNoElementHorizontalOverflow,
  expectLocatorContainedBy,
  requireBox,
} from "../../helpers/layout-assertions";
import { waitForQueuedCount } from "../../helpers/session-resume-prompt-queue";
import {
  CAPACITY_AUTO_RUN,
  CAPACITY_AUTO_RUN_OFF,
  capacityNotice,
  cleanupMessageCapacityFixture,
  expectCollapsedCapacityNotice,
  expectOneAgentReply,
  seedMessageCapacityFixture,
  submitFollowUp,
} from "./message-turn-capacity.helpers";

test.describe("mobile: Message turns wait for session capacity", () => {
  test.describe.configure({ retries: 0 });

  test("keeps one accepted follow-up pending, then delivers it after release", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    isMobile,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const marker = "mobile capacity follow-up delivered";
    const fixture = await seedMessageCapacityFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Mobile message capacity",
    );

    try {
      const submit = fixture.session.submitButton();
      const submitBox = await submit.boundingBox();
      expect(submitBox).not.toBeNull();
      expect(submitBox!.width).toBeGreaterThanOrEqual(44);
      expect(submitBox!.height).toBeGreaterThanOrEqual(44);
      await expect(submit).toBeInViewport();

      await submitFollowUp(fixture, marker, isMobile);
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expectCollapsedCapacityNotice(fixture.session, CAPACITY_AUTO_RUN);
      const notice = capacityNotice(fixture.session);
      const composer = fixture.session.activeChat().getByTestId("chat-input-area");
      const chip = composer.getByTestId("queue-chip");
      await expect(notice).toBeInViewport({ ratio: 1 });
      await assertLocatorWithinViewportX(notice, "capacity notice");
      await expectLocatorContainedBy(notice, composer, "capacity notice in composer");
      await assertNoElementHorizontalOverflow(notice, "capacity notice");
      const [noticeBox, chipBox] = await Promise.all([
        requireBox(notice, "capacity notice"),
        requireBox(chip, "queue chip"),
      ]);
      expect(noticeBox.y + noticeBox.height).toBeLessThanOrEqual(chipBox.y);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile message capacity");

      await testPage.reload();
      await fixture.session.waitForLoad();
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expectCollapsedCapacityNotice(fixture.session, CAPACITY_AUTO_RUN);

      await apiClient.stopSession({
        session_id: fixture.filler.session_id,
        reason: "mobile message capacity e2e release",
        force: true,
      });
      await waitForQueuedCount(apiClient, fixture.identity, 0, 60_000);
      await expect(capacityNotice(fixture.session)).toHaveCount(0);
      await expectOneAgentReply(apiClient, fixture, marker);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile message capacity response");
    } finally {
      await cleanupMessageCapacityFixture(apiClient, fixture);
    }
  });

  test("keeps the accepted follow-up pending while Auto-run is off", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    isMobile,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const marker = "mobile capacity auto-run off";
    const fixture = await seedMessageCapacityFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      "Mobile message capacity auto-run",
    );

    try {
      await expect(apiClient.setQueueAutoRun(fixture.identity, false)).resolves.toMatchObject({
        auto_run: false,
      });
      await submitFollowUp(fixture, marker, isMobile);
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expectCollapsedCapacityNotice(fixture.session, CAPACITY_AUTO_RUN_OFF);

      await apiClient.stopSession({
        session_id: fixture.filler.session_id,
        reason: "mobile message capacity e2e release",
        force: true,
      });
      await dwell(
        testPage,
        2_000,
        "negative-assertion",
        "No event announces that Auto-run OFF intentionally leaves the entry pending",
      );
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expectCollapsedCapacityNotice(fixture.session, CAPACITY_AUTO_RUN_OFF);

      await expect(apiClient.setQueueAutoRun(fixture.identity, true)).resolves.toMatchObject({
        auto_run: true,
      });
      await waitForQueuedCount(apiClient, fixture.identity, 0, 60_000);
      await expect(capacityNotice(fixture.session)).toHaveCount(0);
      await expectOneAgentReply(apiClient, fixture, marker);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile message capacity auto-run");
    } finally {
      await cleanupMessageCapacityFixture(apiClient, fixture);
    }
  });
});
