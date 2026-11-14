// The coordinator's answer card in Needs you: the question, the draft, its
// sources, confidence and the "Needs your decision" badge, and Approve / Reject
// (docs/specs/coordinator/requirements/answer-proposals.md). The phone-sized run
// lives in mobile-answer-proposals.spec.ts because the `mobile-chrome` project
// matches `mobile-*` filenames only.
import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForAgentMessage } from "../../helpers/session";
import {
  ANSWER_DRAFT_TEXT,
  ANSWER_RATIONALE,
  ANSWER_SOURCE_URL,
  setupAnswerProposal,
} from "./answer-proposals-fixture";

const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;
const PROPOSAL_REJECT = /\/proposals\/[^/]+\/reject$/;

async function submitWithShortcut(_popover: Locator, editor: Locator) {
  await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
}

test.describe("Coordinator answer proposals", () => {
  test("the card shows the question, draft, sources and badge, and Approve delivers the answer", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, task, proposalId } = await setupAnswerProposal(submitWithShortcut, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await expect(card).toBeVisible();
      await expect(card.getByText(/Which database should we use/).first()).toBeVisible();
      await expect(card.getByText(new RegExp(ANSWER_DRAFT_TEXT)).first()).toBeVisible();
      await expect(card.getByText(ANSWER_RATIONALE).first()).toBeVisible();
      await expect(card.getByTestId("answer-needs-decision")).toHaveText("Needs your decision");
      await expect(card.getByTestId("answer-confidence")).toContainText("medium");
      await expect(card.getByRole("link", { name: "Architecture doc" })).toHaveAttribute(
        "href",
        ANSWER_SOURCE_URL,
      );

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByRole("button", { name: "Approve" }).click();
      await approved;

      await expect(card).toBeHidden();
      if (!task.session_id) throw new Error("expected a session");
      await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);
    } finally {
      await release();
    }
  });

  test("Reject settles the card and leaves the question unanswered", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, proposalId, workspacePath } = await setupAnswerProposal(submitWithShortcut, {
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await card.getByRole("button", { name: "Reject" }).click();
      const rejected = waitForHttp(testPage, "POST", PROPOSAL_REJECT);
      await card.getByRole("button", { name: "Confirm reject" }).click();
      await rejected;
      await expect(card).toBeHidden();
      const listed = await apiClient.rawRequest("GET", `${workspacePath}/proposals?status=all`);
      const body = (await listed.json()) as {
        proposals: Array<{ id: string; status: string }>;
      };
      expect(body.proposals.find((p) => p.id === proposalId)?.status).toBe("rejected");
    } finally {
      await release();
    }
  });
});
