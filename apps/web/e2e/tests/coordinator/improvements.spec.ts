// AC-COORDINATOR-IMPROVEMENTS-001.x / -002.x / -003.x: the mock agent proposes an
// improvement through the copilot chat with the
// `e2e:mcp:kandev:propose_improvement_kandev` script-mode line (evidence is a
// run row of this coordinator, inserted straight into the e2e database because
// a run cannot be created over HTTP). The card needs the diff shown before
// Approve, approving only records a reviewable change, and a manager Applies or
// Discards it in the coordinator's settings.
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { DatabaseSync } from "../../helpers/node-sqlite";
import { waitForSessionState } from "../../helpers/session";
import {
  linkToCoordinatorAutonomySettings,
  linkToCoordinatorNeedsYou,
} from "../../../lib/coordinator/links";

const BEFORE = "Line one\nLine two";
const AFTER = "Line one\nLine two changed\nLine three";
const TITLE = "Tighten the context";
const RATIONALE = "The context should say what changed.";

type Ctx = Parameters<Parameters<typeof test>[2]>[0];

function insertRun(dbPath: string, coordinatorId: string): string {
  const runId = `run-e2e-${Date.now()}`;
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 10000");
    const at = new Date(Date.now() - 60_000).toISOString();
    db.prepare(
      `INSERT INTO coordinator_unattended_turns
         (id, coordinator_id, conversation_task_id, session_id, wake_count,
          start_ceiling_subcents, outcome, started_at, finished_at)
       VALUES (?, ?, 't', 's', 1, 0, 'completed', ?, ?)`,
    ).run(runId, coordinatorId, at, at);
  } finally {
    db.close();
  }
  return runId;
}

async function proposeImprovement({ testPage, apiClient, backend, seedData }: Ctx) {
  const release = await backend.useEnv({
    KANDEV_FEATURES_COORDINATOR: "true",
    KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
    KANDEV_FEATURES_COORDINATOR_PHASE3: "true",
  });
  const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
    name: "Improvements Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
    context: BEFORE,
  });
  const runId = insertRun(path.join(backend.tmpDir, "kandev.db"), coordinator.id);
  const workspacePath = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;

  await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
  const conversationOpened = waitForHttp(testPage, "POST", /\/coordinators\/[^/]+\/conversation$/);
  await testPage.getByTestId("coordinator-copilot-launcher").click();
  const opened = (await (await conversationOpened).json()) as {
    task_id: string;
    session_id: string;
  };
  const popover = testPage.getByTestId("coordinator-copilot-popover");
  const editor = popover.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  const args = {
    title: TITLE,
    rationale: RATIONALE,
    context: AFTER,
    evidence: [{ run_id: runId }],
  };
  await editor.fill(`e2e:mcp:kandev:propose_improvement_kandev(${JSON.stringify(args)})`);
  await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
  await waitForSessionState(apiClient, {
    taskId: opened.task_id,
    sessionId: opened.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "the mock agent's turn ends after the propose call",
    timeout: 30_000,
  });

  let proposalId = "";
  await expect
    .poll(
      async () => {
        const listed = await apiClient.rawRequest("GET", `${workspacePath}/proposals`);
        const body = (await listed.json()) as { proposals: Array<{ id: string; kind: string }> };
        proposalId = body.proposals.find((p) => p.kind === "improvement")?.id ?? "";
        return proposalId;
      },
      { timeout: 30_000, message: "the improvement proposal should be stored" },
    )
    .not.toBe("");

  await popover.getByRole("button", { name: "Close" }).focus();
  await testPage.keyboard.press("Escape");
  await expect(popover).toBeHidden();
  return { release, coordinator, proposalId, workspacePath };
}

async function approveThroughCard(testPage: Ctx["testPage"], proposalId: string) {
  const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
  await expect(card).toBeVisible();
  await expect(card.getByText(TITLE)).toBeVisible();
  await expect(card.getByText(RATIONALE).first()).toBeVisible();
  await expect(card.getByTestId("improvement-evidence-run")).toBeVisible();
  const approve = card.getByRole("button", { name: "Approve as a reviewable change" });
  await expect(approve).toBeDisabled();
  await expect(card.getByRole("button", { name: "Edit" })).toHaveCount(0);

  await card.getByTestId("improvement-show-change").click();
  const diff = card.getByTestId("context-diff");
  await expect(diff).toContainText("Line two changed");
  await expect(diff).toContainText("+");
  await expect(approve).toBeEnabled();

  const approved = waitForHttp(testPage, "POST", /\/proposals\/[^/]+\/approve$/);
  await approve.click();
  await approved;
  return card;
}

test.describe("Coordinator improvements", () => {
  test("approve records a change that a manager applies in settings", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, coordinator, proposalId, workspacePath } = await proposeImprovement({
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      await approveThroughCard(testPage, proposalId);

      // Approving alone changes nothing.
      const stored = await (await apiClient.rawRequest("GET", workspacePath)).json();
      expect((stored as { context: string }).context).toBe(BEFORE);

      await testPage.goto(linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinator.id));
      const list = testPage.getByTestId("changes-waiting");
      await expect(list).toBeVisible();
      await expect(list.getByText(TITLE)).toBeVisible();
      await expect(list.getByTestId("context-diff")).toContainText("Line three");

      const applied = waitForHttp(testPage, "POST", /\/pending-changes\/[^/]+\/apply$/);
      await list.getByTestId("pending-change-apply").click();
      await applied;

      await expect(testPage.getByTestId("changes-waiting-empty")).toBeVisible();
      await expect
        .poll(
          async () =>
            (
              (await (await apiClient.rawRequest("GET", workspacePath)).json()) as {
                context: string;
              }
            ).context,
          { timeout: 15_000, message: "Apply should write the proposed context" },
        )
        .toBe(AFTER);
    } finally {
      await release();
    }
  });

  test("discarding a change leaves the context untouched", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, coordinator, proposalId, workspacePath } = await proposeImprovement({
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      await approveThroughCard(testPage, proposalId);
      await testPage.goto(linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinator.id));
      const list = testPage.getByTestId("changes-waiting");
      await expect(list.getByText(TITLE)).toBeVisible();

      const discarded = waitForHttp(testPage, "POST", /\/pending-changes\/[^/]+\/discard$/);
      await list.getByTestId("pending-change-discard").click();
      await discarded;

      await expect(testPage.getByTestId("changes-waiting-empty")).toBeVisible();
      const stored = await (await apiClient.rawRequest("GET", workspacePath)).json();
      expect((stored as { context: string }).context).toBe(BEFORE);
    } finally {
      await release();
    }
  });

  test("rejecting an improvement stores no change", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const { release, coordinator, proposalId, workspacePath } = await proposeImprovement({
      testPage,
      apiClient,
      backend,
      seedData,
    });
    try {
      const card = testPage.getByTestId(`needs-you-item-${proposalId}`);
      await card.getByRole("button", { name: "Reject" }).click();
      const rejected = waitForHttp(testPage, "POST", /\/proposals\/[^/]+\/reject$/);
      await card.getByRole("button", { name: "Confirm reject" }).click();
      await rejected;

      const changes = await apiClient.rawRequest("GET", `${workspacePath}/pending-changes`);
      expect(((await changes.json()) as { changes: unknown[] }).changes).toHaveLength(0);
      await testPage.goto(linkToCoordinatorAutonomySettings(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId("changes-waiting-empty")).toBeVisible();
      const stored = await (await apiClient.rawRequest("GET", workspacePath)).json();
      expect((stored as { context: string }).context).toBe(BEFORE);
    } finally {
      await release();
    }
  });
});
