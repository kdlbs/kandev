// AC-COORDINATOR-PROPOSAL-KINDS-004.x on a phone-sized viewport: the move card's
// actions are 44px touch targets, the page does not scroll sideways, and a tap
// on Approve settles it through the phase-1 route. The `mobile-chrome` project
// matches this filename and applies the Pixel 5 emulation.
import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { setupMoveProposal } from "./proposal-kinds-fixture";

const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const MIN_TOUCH_TARGET_PX = 44;

// Enter inserts a newline on a coarse pointer; the inline Send button submits.
async function submitWithTap(popover: Locator) {
  await popover.getByTestId("submit-message-button").tap();
}

test.describe("Coordinator move card on a phone viewport", () => {
  test("actions are touch sized, nothing overflows, and Approve moves the task", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, toStep, proposalId } = await setupMoveProposal(submitWithTap, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await expect(card).toBeVisible();
      for (const name of ["Approve", "Reject"]) {
        const box = await card.getByRole("button", { name }).boundingBox();
        expect(box, `${name} should have a box`).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      }
      const overflow = await testPage.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByRole("button", { name: "Approve" }).tap();
      await approved;
      await expect
        .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, { timeout: 15_000 })
        .toBe(toStep.id);
    } finally {
      await release();
    }
  });
});
