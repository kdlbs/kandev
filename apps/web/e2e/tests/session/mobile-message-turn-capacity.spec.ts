// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForQueuedCount } from "../../helpers/session-resume-prompt-queue";
import {
  cleanupMessageCapacityFixture,
  seedMessageCapacityFixture,
  submitFollowUp,
  waitForAgentMessage,
} from "./message-turn-capacity.helpers";

test.describe("mobile: Message turns wait for session capacity", () => {
  test.describe.configure({ retries: 0 });

  test("keeps one accepted follow-up pending, then delivers it after release", async ({
    testPage,
    apiClient,
    seedData,
    backend,
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

      await submitFollowUp(testPage, fixture, marker);
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expect(fixture.session.activeChat().getByTestId("queue-chip")).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile message capacity");

      await testPage.reload();
      await fixture.session.waitForLoad();
      await waitForQueuedCount(apiClient, fixture.identity, 1);
      await expect(fixture.session.activeChat().getByTestId("queue-chip")).toBeVisible();

      await apiClient.stopSession({
        session_id: fixture.filler.session_id,
        reason: "mobile message capacity e2e release",
        force: true,
      });
      await waitForQueuedCount(apiClient, fixture.identity, 0, 60_000);
      await waitForAgentMessage(apiClient, fixture.target.session_id, marker);
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
      await submitFollowUp(testPage, fixture, marker);
      await waitForQueuedCount(apiClient, fixture.identity, 1);

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

      await expect(apiClient.setQueueAutoRun(fixture.identity, true)).resolves.toMatchObject({
        auto_run: true,
      });
      await waitForQueuedCount(apiClient, fixture.identity, 0, 60_000);
      await waitForAgentMessage(apiClient, fixture.target.session_id, marker);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile message capacity auto-run");
    } finally {
      await cleanupMessageCapacityFixture(apiClient, fixture);
    }
  });
});
