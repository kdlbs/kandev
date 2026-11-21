// The answer card on a phone-sized viewport: its actions are 44px touch targets,
// the page does not scroll sideways, and a tap on Approve settles it. The
// `mobile-chrome` project matches this filename and applies the Pixel 5 emulation.
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { setupAnswerProposal } from "./answer-proposals-fixture";

const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const MIN_TOUCH_TARGET_PX = 44;

// Enter inserts a newline on a coarse pointer; the inline Send button submits.
async function submitWithTap(popover: Locator) {
  await popover.getByTestId("submit-message-button").tap();
}

async function expectNoSidewaysScroll(page: Page) {
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
}

test.describe("Coordinator answer card on a phone viewport", () => {
  test("actions are touch sized, nothing overflows, and Approve settles the card", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, proposalId } = await setupAnswerProposal(submitWithTap, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await expect(card).toBeVisible();
      for (const name of ["Approve", "Edit", "Reject"]) {
        const box = await card.getByRole("button", { name }).boundingBox();
        expect(box, `${name} should have a box`).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      }
      await expect(card.getByTestId("answer-needs-decision")).toBeVisible();
      await expectNoSidewaysScroll(testPage);

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByRole("button", { name: "Approve" }).tap();
      await approved;
      await expect(card).toBeHidden();
    } finally {
      await release();
    }
  });
});
