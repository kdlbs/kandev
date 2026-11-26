// Coordinator merge stewardship UI on a phone viewport. The `mobile-chrome` project
// matches this filename and applies the Pixel 5 emulation. Merge reads are stubbed at the
// network edge (see merge-fixture.ts).
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";
import {
  GREEN_LIVE,
  enableMergeFeatures,
  mergeProposalBody,
  newSettingsStub,
  stubMergeProposal,
  stubMergeSettings,
  type ProposalStub,
} from "./merge-fixture";

const MIN_TOUCH_TARGET_PX = 44;
const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;

async function expectNoSidewaysScroll(page: Page) {
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
}

async function expectTouchSized(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} should have a box`).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
}

async function makeCoordinator(
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string },
  name: string,
) {
  return apiClient.createCoordinator(seedData.workspaceId, {
    name,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    task_agent_profile_id: seedData.agentProfileId,
    task_executor_profile_id: seedData.worktreeExecutorProfileId,
  });
}

test.describe("Merge on a phone viewport", () => {
  test("the settings section stacks, its controls are touch sized, and a tap changes the policy", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Phone Merge Settings");
      const stub = newSettingsStub(
        [
          {
            repository_id: "repo-1",
            name: "billing-service",
            policy: "denied",
            automatic_active: true,
          },
          {
            repository_id: "repo-2",
            name: "web-app",
            policy: "automatic",
            automatic_active: false,
          },
        ],
        [
          {
            repository_id: "repo-1",
            branch: "main",
            pipeline: "unknown",
            approvals: "enforced",
            threads: "not_enforced",
            observed_at: "2026-05-01T10:00:00Z",
          },
        ],
      );
      await stubMergeSettings(testPage, coordinator.id, stub);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=merge`,
      );

      const section = testPage.getByTestId("merge-section");
      await expect(section).toBeVisible();
      await expectTouchSized(section.getByTestId("merge-protection-refresh"), "Refresh protection");
      await expectTouchSized(
        section.getByTestId("merge-replace-repo-2"),
        "Replace execution manager",
      );
      const width = await section.getByTestId("merge-replace-repo-2").evaluate((el) => ({
        button: el.getBoundingClientRect().width,
        parent: el.parentElement!.getBoundingClientRect().width,
      }));
      expect(width.button).toBeGreaterThan(width.parent * 0.9);
      await expectNoSidewaysScroll(testPage);

      await section.getByTestId("merge-policy-repo-1").getByLabel("Requires approval").tap();
      await expect(
        section.getByTestId("merge-policy-repo-1").getByLabel("Requires approval"),
      ).toBeChecked();
    } finally {
      await release();
    }
  });

  test("the merge card actions span the card, are touch sized, and Approve settles once", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Phone Merge Card");
      const stub: ProposalStub = {
        proposal: mergeProposalBody(coordinator.id, seedData.workspaceId, "task-x"),
        detail: {
          change_key: "github|github.com|repo-1|601",
          repository_id: "repo-1",
          head_sha: "0123456789abcdef",
          method: "squash",
          automatic: false,
          manager_only: false,
          live: GREEN_LIVE,
        },
        approves: 0,
        checks: 0,
        releases: 0,
      };
      await stubMergeProposal(testPage, coordinator.id, stub);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

      const card = testPage.getByTestId("needs-you-item-merge-proposal-1");
      await expect(card).toBeVisible();
      await expect(card.getByTestId("merge-cannot-undo")).toBeVisible();
      await expectTouchSized(card.getByTestId("merge-approve"), "Approve merge");
      await expectTouchSized(card.getByTestId("merge-reject"), "Reject");
      await expectNoSidewaysScroll(testPage);

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByTestId("merge-approve").tap();
      await approved;
      await expect.poll(() => stub.approves).toBe(1);
    } finally {
      await release();
    }
  });

  test("an uncertain merge shows Check now touch sized and no Approve", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Phone Merge Uncertain");
      const stub: ProposalStub = {
        proposal: mergeProposalBody(coordinator.id, seedData.workspaceId, "task-x", {
          status: "uncertain",
        }),
        detail: {
          change_key: "github|github.com|repo-1|601",
          repository_id: "repo-1",
          head_sha: "0123456789abcdef",
          method: "squash",
          automatic: true,
          manager_only: false,
          dispatch: {
            outcome: "uncertain",
            queued: false,
            dispatched_at: new Date().toISOString(),
            can_check: true,
            can_release: true,
          },
        },
        approves: 0,
        checks: 0,
        releases: 0,
      };
      await stubMergeProposal(testPage, coordinator.id, stub);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

      const card = testPage.getByTestId("needs-you-item-merge-proposal-1");
      await expect(card.getByTestId("merge-card-uncertain")).toBeVisible();
      await expect(card.getByTestId("merge-approve")).toHaveCount(0);
      await expectTouchSized(card.getByTestId("merge-check-now"), "Check now");
      await expectTouchSized(card.getByTestId("merge-release"), "Release");
      await expectNoSidewaysScroll(testPage);

      const checked = waitForHttp(testPage, "POST", /\/merge\/check$/);
      await card.getByTestId("merge-check-now").tap();
      await checked;
      expect(stub.checks).toBe(1);
    } finally {
      await release();
    }
  });
});
