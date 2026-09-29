// AC-COORDINATOR-PROPOSAL-KINDS-004.x / -005.x and STANDING-ORDERS-003.x: the
// move card approves through the phase-1 route, and a reject with a reason
// offers "Make it a standing order" (scenario 05-rule-on-a-proposal).
// A proposal cannot be seeded over HTTP; the mock agent creates it through the
// coordinator's copilot chat with the `e2e:mcp:kandev:propose_move_kandev`
// script-mode line. The phone-sized run lives in mobile-proposal-kinds.spec.ts
// because the `mobile-chrome` project matches `mobile-*` filenames only.
import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { setupMoveProposal } from "./proposal-kinds-fixture";

const PROPOSAL_REJECT = /\/proposals\/[^/]+\/reject$/;
const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const STANDING_ORDER_ADD = /\/standing-orders$/;

async function submitWithShortcut(_popover: Locator, editor: Locator) {
  await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
}

test.describe("Coordinator proposal kinds", () => {
  test("a move card approves in Needs you and the task changes step", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, toStep, proposalId } = await setupMoveProposal(submitWithShortcut, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await expect(card).toBeVisible();
      await expect(card.getByText("It is ready for review.").first()).toBeVisible();
      expect((await apiClient.getTask(task.id)).workflow_step_id).not.toBe(toStep.id);

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByRole("button", { name: "Approve" }).click();
      await approved;

      await expect(card).toBeHidden();
      await expect
        .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, { timeout: 15_000 })
        .toBe(toStep.id);
    } finally {
      await release();
    }
  });

  test("reject with a reason offers a standing order that is saved", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, proposalId, workspacePath } = await setupMoveProposal(submitWithShortcut, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await card.getByRole("button", { name: "Reject" }).click();
      await card.getByLabel("Reason (optional)").fill("Never move tasks out of triage.");
      const rejected = waitForHttp(testPage, "POST", PROPOSAL_REJECT);
      await card.getByRole("button", { name: "Confirm reject" }).click();
      await rejected;

      const offer = testPage.getByRole("button", { name: "Make it a standing order" });
      await expect(offer).toBeVisible();
      await offer.click();
      const dialog = testPage.getByTestId("add-standing-order-dialog");
      await expect(dialog.getByRole("textbox")).toHaveValue("Never move tasks out of triage.");

      const added = waitForHttp(testPage, "POST", STANDING_ORDER_ADD);
      await dialog.getByRole("button", { name: "Add", exact: true }).click();
      await added;
      await expect(testPage.getByText("Standing order added.")).toBeVisible();

      const listed = await apiClient.rawRequest("GET", `${workspacePath}/standing-orders`);
      const body = (await listed.json()) as {
        orders: Array<{ text: string }>;
      };
      expect(body.orders).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ text: "Never move tasks out of triage." }),
        ]),
      );
    } finally {
      await release();
    }
  });
});
