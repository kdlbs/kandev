// Coordinator merge stewardship UI on a desktop viewport: the Merge settings section,
// the merge proposal card (pending, uncertain), and the Queue change state and
// predecessor editor. Merge reads are stubbed at the network edge (see merge-fixture.ts).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorQueue,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";
import {
  GREEN_LIVE,
  changeBody,
  enableMergeFeatures,
  mergeProposalBody,
  newSettingsStub,
  stubMergeProposal,
  stubMergeSettings,
  stubQueueMerge,
  type ProposalStub,
  type QueueStub,
} from "./merge-fixture";
import { setupReadyToMergeTask } from "./proposal-kinds-fixture";

const SETTINGS_READ = /\/coordinators\/[^/]+\/settings$/;
const PROPOSAL_APPROVE = /\/proposals\/[^/]+\/approve$/;

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

function repos() {
  return [
    {
      repository_id: "repo-1",
      name: "billing-service",
      policy: "denied" as const,
      automatic_active: true,
    },
    {
      repository_id: "repo-2",
      name: "web-app",
      policy: "automatic" as const,
      automatic_active: false,
    },
  ];
}

const OBSERVED = {
  repository_id: "repo-1",
  branch: "main",
  pipeline: "enforced" as const,
  approvals: "unknown" as const,
  threads: "not_enforced" as const,
  observed_at: "2026-05-01T10:00:00Z",
};

test.describe("Merge settings section", () => {
  test("lists repositories, shows protection, and saves a policy change with its revision", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Merge Settings Coordinator");
      const stub = newSettingsStub(repos(), [OBSERVED]);
      await stubMergeSettings(testPage, coordinator.id, stub);

      const settingsRead = waitForHttp(testPage, "GET", SETTINGS_READ);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=merge`,
      );
      await settingsRead;

      const section = testPage.getByTestId("merge-section");
      await expect(section).toBeVisible();
      await expect(section.getByTestId("merge-statements")).toContainText(
        "merge queue is not supported",
      );
      await expect(section.getByTestId("merge-repo-repo-1")).toContainText("billing-service");
      const cells = section.getByTestId("merge-protection-cell");
      await expect(cells).toHaveCount(3);
      await expect(cells.nth(1)).toHaveAttribute("data-value", "unknown");

      await section.getByTestId("merge-policy-repo-1").getByLabel("Requires approval").click();
      await testPage.getByRole("button", { name: /^Save/ }).click();
      await expect.poll(() => stub.puts.length).toBe(1);
      expect(stub.puts[0]).toMatchObject({
        policy_revision: 1,
        merge: { repositories: [{ repository_id: "repo-1", policy: "requires_approval" }] },
      });
    } finally {
      await release();
    }
  });

  test("refreshes protection and shows a failed load with Retry", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Merge Retry Coordinator");
      const stub = newSettingsStub(repos(), [OBSERVED]);
      stub.readFailing = true;
      await stubMergeSettings(testPage, coordinator.id, stub);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=merge`,
      );
      await expect(testPage.getByTestId("merge-load-error")).toBeVisible();
      stub.readFailing = false;
      await testPage.getByTestId("merge-load-error").getByRole("button", { name: "Retry" }).click();
      await expect(testPage.getByTestId("merge-section")).toBeVisible();

      const refreshed = waitForHttp(testPage, "POST", /\/merge\/protection\/refresh$/);
      await testPage.getByTestId("merge-protection-refresh").click();
      await refreshed;
      expect(stub.refreshes).toBe(1);
    } finally {
      await release();
    }
  });

  test("Replace execution manager is sent on save and a valid principal says so", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Merge Principal Coordinator");
      const stub = newSettingsStub(repos());
      await stubMergeSettings(testPage, coordinator.id, stub);
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=merge`,
      );
      await expect(testPage.getByTestId("merge-replace-repo-1")).toHaveCount(0);

      stub.putFailure = "principal_valid";
      await testPage.getByTestId("merge-replace-repo-2").click();
      await testPage.getByRole("button", { name: /^Save/ }).click();
      await expect(testPage.getByTestId("merge-save-error")).toContainText("still authorized");

      await testPage.getByRole("button", { name: "Retry save" }).click();
      await expect.poll(() => stub.puts.length).toBe(2);
      expect(stub.puts[1]).toMatchObject({
        merge: {
          repositories: [{ repository_id: "repo-2", policy: "automatic", replace_principal: true }],
        },
      });
      await expect(testPage.getByTestId("merge-replace-repo-2")).toHaveCount(0);
    } finally {
      await release();
    }
  });
});

function proposalStub(
  coordinatorId: string,
  workspaceId: string,
  taskId: string,
  over = {},
  detail = {},
) {
  const stub: ProposalStub = {
    proposal: mergeProposalBody(coordinatorId, workspaceId, taskId, over),
    detail: {
      change_key: "github|github.com|repo-1|601",
      repository_id: "repo-1",
      head_sha: "0123456789abcdef",
      method: "squash",
      automatic: false,
      manager_only: false,
      live: GREEN_LIVE,
      ...detail,
    },
    approves: 0,
    checks: 0,
    releases: 0,
  };
  return stub;
}

test.describe("Merge proposal card", () => {
  test("shows conditions, the manager-only note and no Undo, and approves once on a double click", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Merge Card Coordinator");
      const stub = proposalStub(
        coordinator.id,
        seedData.workspaceId,
        "task-x",
        {},
        {
          manager_only: true,
          note: "Daily limit reached",
        },
      );
      await stubMergeProposal(testPage, coordinator.id, stub);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

      const card = testPage.getByTestId("needs-you-item-merge-proposal-1");
      await expect(card).toBeVisible();
      await expect(card.getByTestId("merge-card-title")).toContainText("601");
      await expect(card.getByTestId("merge-card-note")).toHaveText("Daily limit reached");
      await expect(card.getByTestId("merge-conditions")).toContainText("Green");
      await expect(card.getByTestId("merge-cannot-undo")).toHaveText(
        "A merge cannot be undone here.",
      );
      await expect(card.getByRole("button", { name: /undo/i })).toHaveCount(0);

      const approved = waitForHttp(testPage, "POST", PROPOSAL_APPROVE);
      await card.getByTestId("merge-approve").dblclick();
      await approved;
      await expect.poll(() => stub.approves).toBe(1);
    } finally {
      await release();
    }
  });

  test("an uncertain merge offers Check now, no Approve, and Release only after 24 hours", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await enableMergeFeatures(backend);
    try {
      const coordinator = await makeCoordinator(apiClient, seedData, "Merge Uncertain Coordinator");
      const stub = proposalStub(
        coordinator.id,
        seedData.workspaceId,
        "task-x",
        { status: "uncertain" },
        {
          live: undefined,
          dispatch: {
            outcome: "uncertain",
            queued: false,
            dispatched_at: new Date().toISOString(),
            can_check: true,
            can_release: false,
          },
        },
      );
      await stubMergeProposal(testPage, coordinator.id, stub);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

      const card = testPage.getByTestId("needs-you-item-merge-proposal-1");
      await expect(card.getByTestId("merge-card-uncertain")).toBeVisible();
      await expect(card.getByTestId("merge-approve")).toHaveCount(0);
      await expect(card.getByTestId("merge-release")).toHaveCount(0);

      const checked = waitForHttp(testPage, "POST", /\/merge\/check$/);
      await card.getByTestId("merge-check-now").click();
      await checked;
      expect(stub.checks).toBe(1);

      (stub.detail.dispatch as Record<string, unknown>).can_release = true;
      await card.getByTestId("merge-check-now").click();
      await expect(card.getByTestId("merge-release")).toBeVisible();
      const released = waitForHttp(testPage, "POST", /\/merge\/release$/);
      await card.getByTestId("merge-release").click();
      await released;
      expect(stub.releases).toBe(1);
    } finally {
      await release();
    }
  });
});

test.describe("Queue change state and predecessors", () => {
  test("shows every change field, retries a failed read, and refuses duplicate and self predecessors", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, coordinatorId } = await setupReadyToMergeTask({
      apiClient,
      backend,
      seedData,
    });
    const releaseMerge = await enableMergeFeatures(backend);
    try {
      const stub: QueueStub = {
        changes: [changeBody()],
        changeReads: 0,
        retryReads: 0,
        deps: { refs: [{ task: "task-a" }], revision: 1 },
        depPuts: [],
        staleOnce: null,
      };
      await stubQueueMerge(testPage, coordinatorId, task.id, stub);
      await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinatorId));

      await testPage.getByTestId(`queue-changes-toggle-${task.id}`).click();
      const change = testPage.getByTestId("queue-change-601");
      await expect(change).toContainText("main");
      await expect(change).toContainText("0123456");
      await expect(change).toContainText("3 commits behind");
      await expect(change).toContainText("No checks");

      stub.changes = [changeBody({ unavailable: true })];
      const retried = waitForHttp(testPage, "GET", /\/changes$/, {
        predicate: (r) => r.url().includes("retry=1"),
      });
      await testPage.getByTestId(`queue-changes-toggle-${task.id}`).click();
      await testPage.getByTestId(`queue-changes-toggle-${task.id}`).click();
      await expect(testPage.getByTestId("queue-change-failed-601")).toBeVisible();
      await expect(change).toContainText("main");
      await testPage
        .getByTestId("queue-change-failed-601")
        .getByRole("button", { name: "Retry" })
        .click();
      await retried;
      expect(stub.retryReads).toBe(1);

      await testPage.getByTestId(`queue-deps-toggle-${task.id}`).click();
      const editor = testPage.getByTestId(`queue-deps-${task.id}`);
      await expect(editor).toContainText("task-a");
      await testPage.getByTestId("queue-deps-input").fill("task-a");
      await testPage.getByTestId("queue-deps-add").click();
      await expect(testPage.getByTestId("queue-deps-message")).toContainText("already listed");
      await testPage.getByTestId("queue-deps-input").fill(task.id);
      await testPage.getByTestId("queue-deps-add").click();
      await expect(testPage.getByTestId("queue-deps-message")).toContainText("own predecessor");
      expect(stub.depPuts).toHaveLength(0);
    } finally {
      await releaseMerge();
      await release();
    }
  });

  test("a stale predecessor save shows the conflict and reloads the stored set", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { release, task, coordinatorId } = await setupReadyToMergeTask({
      apiClient,
      backend,
      seedData,
    });
    const releaseMerge = await enableMergeFeatures(backend);
    try {
      const stub: QueueStub = {
        changes: [changeBody()],
        changeReads: 0,
        retryReads: 0,
        deps: { refs: [], revision: 1 },
        depPuts: [],
        staleOnce: { refs: [{ task: "task-z" }], revision: 2 },
      };
      await stubQueueMerge(testPage, coordinatorId, task.id, stub);
      await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinatorId));
      await testPage.getByTestId(`queue-deps-toggle-${task.id}`).click();
      await testPage.getByTestId("queue-deps-input").fill("task-b");
      await testPage.getByTestId("queue-deps-add").click();
      await testPage.getByTestId("queue-deps-save").click();
      await expect(testPage.getByTestId("queue-deps-message")).toContainText("changed elsewhere");
      await expect(testPage.getByTestId(`queue-deps-${task.id}`)).toContainText("task-z");
      expect(stub.depPuts[0]).toMatchObject({ dependency_revision: 1, refs: [{ task: "task-b" }] });
    } finally {
      await releaseMerge();
      await release();
    }
  });
});
