// AC-COORDINATOR-PROPOSAL-KINDS-004.x on a phone-sized viewport: the move card's
// actions are 44px touch targets, the page does not scroll sideways, and a tap
// on Approve settles it through the phase-1 route. The `mobile-chrome` project
// matches this filename and applies the Pixel 5 emulation.
import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorQueue } from "../../../lib/coordinator/links";
import {
  MESSAGE_PROPOSAL_TEXT,
  setupMoveProposal,
  setupReadyToMergeTask,
} from "./proposal-kinds-fixture";

const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const MIN_TOUCH_TARGET_PX = 44;

async function expectTouchSized(scope: Locator, names: Array<string | RegExp>) {
  for (const name of names) {
    const box = await scope.getByRole("button", { name }).first().boundingBox();
    expect(box, `${name} should have a box`).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
  }
}

async function expectNoSidewaysScroll(page: import("@playwright/test").Page) {
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
}

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

test.describe("Coordinator message card and Ready to merge row on a phone viewport", () => {
  test("the message card is touch sized and Edit then Approve reaches the task", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, proposalId } = await setupMoveProposal(
      submitWithTap,
      { testPage, apiClient, backend, seedData },
      "message",
    );
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await expect(card.getByText(MESSAGE_PROPOSAL_TEXT).first()).toBeVisible();
      await expectTouchSized(card, ["Approve", "Edit", "Reject"]);
      await expectNoSidewaysScroll(testPage);

      await card.getByRole("button", { name: "Edit" }).tap();
      const edited = "Please run the linter and the tests before you finish.";
      await card.getByRole("textbox").fill(edited);
      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByRole("button", { name: /Approve/ }).tap();
      await approved;
      await expect
        .poll(
          async () =>
            (await apiClient.listSessionMessages(task.session_id!)).messages.some(
              (message) => message.author_type === "user" && message.content.includes(edited),
            ),
          { timeout: 30_000, message: "the edited text should reach the task's conversation" },
        )
        .toBe(true);
    } finally {
      await release();
    }
  });

  test("the Ready to merge row is touch sized and Send it back queues a note", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, sessionId, coordinatorId } = await setupReadyToMergeTask({
      apiClient,
      backend,
      seedData,
    });
    try {
      await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinatorId));
      const row = testPage.getByTestId(`queue-ready-row-${task.id}`);
      await expect(row).toBeVisible();
      const openPr = await row.getByRole("link", { name: "Open the PR" }).boundingBox();
      expect(openPr!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      await expectTouchSized(row, ["Send it back"]);
      await expectNoSidewaysScroll(testPage);

      await row.getByRole("button", { name: "Send it back" }).tap();
      const note = "Please add a regression test before merging.";
      await row.getByRole("textbox").fill(note);
      await expectTouchSized(row, [/^Send$/, "Cancel"]);
      await expectNoSidewaysScroll(testPage);
      await row.getByRole("button", { name: "Send", exact: true }).tap();
      await expect(row.getByRole("textbox")).toBeHidden();
      await expect
        .poll(
          async () =>
            (await apiClient.listSessionMessages(sessionId)).messages.some(
              (message) => message.author_type === "user" && message.content.includes(note),
            ),
          { timeout: 30_000, message: "the note should reach the task's session" },
        )
        .toBe(true);
    } finally {
      await release();
    }
  });
});
