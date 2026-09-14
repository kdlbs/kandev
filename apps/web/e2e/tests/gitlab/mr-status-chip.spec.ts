import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * Seeds one open MR with a successful pipeline and 0/1 approvals
 * (-> awaiting_approval per the chip's state machine). Deliberately does NOT
 * reuse the shared seedGitLabReview helper (e2e/helpers/gitlab.ts): that
 * helper seeds GitLab's raw wording state: "opened", which MRTopbarButton
 * doesn't care about (it renders on mrs.length > 0 regardless of state) but
 * the chip's spec-mandated `state === "open"` exact-equality filter does not
 * match. The mock provider never normalizes "opened" -> "open" (only the
 * real GitLab HTTP client's convertRawMR does that), so a chip-focused test
 * must seed the already-normalized value directly — the same convention
 * mr-task-card-badge.spec.ts's local seedMR helper already uses for the same
 * reason.
 */
async function seedChipMR(apiClient: ApiClient, workspaceId: string, iid: number, title: string) {
  await apiClient.configureGitLab(workspaceId, GITLAB_HOST);
  await apiClient.mockGitLabAddMRs(workspaceId, GITLAB_PROJECT, [mrSeed(iid, title)]);
  await apiClient.mockGitLabAddPipelines(workspaceId, GITLAB_PROJECT, [
    pipelineSeed(iid, "success"),
  ]);
  await apiClient.mockGitLabAddApprovals(workspaceId, GITLAB_PROJECT, iid, [], 1);
}

/** Create a passthrough agent profile, mirroring cli-mode/passthrough-toolbar.spec.ts. */
async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
  const profile = await apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
  return profile.id;
}

async function createTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function linkMR(apiClient: ApiClient, seedData: SeedData, taskId: string, iid: number) {
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: taskId,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
  });
}

async function openTask(
  testPage: import("@playwright/test").Page,
  session: SessionPage,
  taskId: string,
  options: { expectedMrCount?: number } = {},
) {
  await testPage.goto(`/t/${taskId}`);
  await session.waitForLoad();
  // The shell hydrates the workspace MR map once per document. A link created
  // immediately before navigation can miss that first snapshot even though
  // the task details already show the association. Re-drive document
  // hydration until the linked-MR chip observes the persisted map; one fixed
  // reload can race the same snapshot again under CI load.
  await expect(async () => {
    await testPage.reload();
    await session.waitForLoad();
    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 5_000 });
    if (options.expectedMrCount !== undefined) {
      await expect(chip).toHaveAttribute("data-mr-count", String(options.expectedMrCount), {
        timeout: 5_000,
      });
    }
  }).toPass({ timeout: 30_000 });
}

test.describe("GitLab MR status chip", () => {
  test("renders in the chat status bar for a single open MR, and hovering reveals the popover body", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 401;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Chip render MR");
    const task = await createTask(apiClient, seedData, "MR chip render");
    await linkMR(apiClient, seedData, task.id, MR_IID);

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id);

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    // seedChipMR: pipeline succeeds, approvals 0/1 -> awaiting_approval.
    await expect(chip).toHaveAttribute("data-status", "awaiting_approval");
    await expect(chip).toHaveAttribute("data-mr-count", "1");
    await expect(chip).toHaveAttribute("data-mr-iid", String(MR_IID));
    await expect(chip).toHaveAttribute("data-selection-frozen", "false");

    await chip.hover();
    const inner = session.mrStatusChipPopoverInner();
    await expect(inner).toBeVisible({ timeout: 5_000 });
    await expect(inner).toContainText(`!${MR_IID}`);

    const box = await inner.boundingBox();
    expect(box?.y).toBeGreaterThanOrEqual(0);
  });

  test("selects the awaiting-approval MR over a draft one (higher rank), and freezes the acted-on MR while the popover is open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const DRAFT_IID = 402;
    const AWAITING_IID = 403;
    await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
    // The mock provider's pipeline list is keyed by project only (shared by
    // every MR in it, not per-ref), so both MRs get the same successful
    // pipeline here and are differentiated by draft/approval state instead.
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      { ...mrSeed(DRAFT_IID, "Draft MR"), draft: true },
      mrSeed(AWAITING_IID, "Awaiting approval MR"),
    ]);
    await apiClient.mockGitLabAddPipelines(seedData.workspaceId, GITLAB_PROJECT, [
      pipelineSeed(DRAFT_IID, "success"),
    ]);
    await apiClient.mockGitLabAddApprovals(
      seedData.workspaceId,
      GITLAB_PROJECT,
      AWAITING_IID,
      [],
      1,
    );

    const task = await createTask(apiClient, seedData, "MR chip multi-select");
    await linkMR(apiClient, seedData, task.id, DRAFT_IID);
    await linkMR(apiClient, seedData, task.id, AWAITING_IID);

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id, { expectedMrCount: 2 });

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await expect(chip).toHaveAttribute("data-status", "awaiting_approval");
    await expect(chip).toHaveAttribute("data-mr-count", "2");
    await expect(chip).toHaveAttribute("data-mr-iid", String(AWAITING_IID));

    await chip.hover();
    await expect(session.mrStatusChipPopoverInner()).toBeVisible({ timeout: 5_000 });
    await expect(chip).toHaveAttribute("data-selection-frozen", "true");
    // The frozen acted-on MR keeps naming the higher-ranked MR while the popover stays open.
    await expect(chip).toHaveAttribute("data-mr-iid", String(AWAITING_IID));
  });

  test("unlinking from the chip popover removes the association and the chip unmounts", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 404;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Unlink MR");
    const task = await createTask(apiClient, seedData, "MR chip unlink");
    await linkMR(apiClient, seedData, task.id, MR_IID);

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id);

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await chip.hover();
    const inner = session.mrStatusChipPopoverInner();
    await expect(inner).toBeVisible({ timeout: 5_000 });

    await inner.getByRole("button", { name: `Unlink !${MR_IID}` }).click();
    await expect(chip).toHaveCount(0, { timeout: 10_000 });
  });

  test("activating link-another closes the popover before the link dialog opens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 405;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Link another MR");
    const task = await createTask(apiClient, seedData, "MR chip link another");
    await linkMR(apiClient, seedData, task.id, MR_IID);

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id);

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await chip.hover();
    const inner = session.mrStatusChipPopoverInner();
    await expect(inner).toBeVisible({ timeout: 5_000 });

    await inner.getByTestId("mr-popover-link-another").click();
    await expect(session.mrStatusChipPopoverInner()).toBeHidden({ timeout: 5_000 });
    await expect(testPage.getByRole("dialog")).toBeVisible({ timeout: 5_000 });
  });

  test("renders auto-fix (round 0) and auto-merge badges once automation is enabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 406;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Automation badges MR");
    const task = await createTask(apiClient, seedData, "MR chip automation badges");
    await linkMR(apiClient, seedData, task.id, MR_IID);
    await apiClient.updateTaskMRAutomationOptions(task.id, {
      auto_fix_enabled: true,
      auto_merge_enabled: true,
    });

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id);

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    const autoFixBadge = chip.getByTestId("mr-status-auto-fix-chip");
    await expect(autoFixBadge).toBeVisible();
    await expect(autoFixBadge).toContainText("0/10");
    await expect(autoFixBadge).toHaveAttribute("data-auto-fix-exhausted", "false");
    await expect(chip.getByTestId("mr-status-auto-merge-chip")).toBeVisible();
  });

  // The switches are per MR, so the response's top-level booleans are an
  // aggregate ("on for every linked MR"). The chip used to read that
  // aggregate, so enabling automation on one of two linked MRs rendered no
  // badge row at all (mr-status-chip-trigger.tsx returns null when both
  // flags are false) while auto-fix was genuinely running on that MR.
  test("renders the badges when only one of two linked MRs has automation on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const ARMED_IID = 410;
    const IDLE_IID = 411;
    // Configure once, then seed both MRs: each configureGitLab call rebuilds
    // the workspace's cached mock client and discards MRs seeded before it.
    await apiClient.configureGitLab(seedData.workspaceId, GITLAB_HOST);
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      mrSeed(ARMED_IID, "Armed MR"),
      mrSeed(IDLE_IID, "Idle MR"),
    ]);
    await apiClient.mockGitLabAddPipelines(seedData.workspaceId, GITLAB_PROJECT, [
      pipelineSeed(ARMED_IID, "success"),
    ]);
    await apiClient.mockGitLabAddApprovals(seedData.workspaceId, GITLAB_PROJECT, ARMED_IID, [], 1);
    await apiClient.mockGitLabAddApprovals(seedData.workspaceId, GITLAB_PROJECT, IDLE_IID, [], 1);

    const task = await createTask(apiClient, seedData, "MR chip per-MR badges");
    await linkMR(apiClient, seedData, task.id, ARMED_IID);
    await linkMR(apiClient, seedData, task.id, IDLE_IID);

    await apiClient.updateTaskMRAutomationOptions(task.id, {
      repository_id: seedData.repositoryId,
      project_path: GITLAB_PROJECT,
      mr_iid: ARMED_IID,
      auto_fix_enabled: true,
      auto_merge_enabled: true,
    });

    // Pin the precondition: the aggregate the chip used to read is false,
    // so a passing assertion below cannot come from the old code path.
    const options = await apiClient.getTaskMRAutomationOptions(task.id);
    expect(options.auto_fix_enabled).toBe(false);
    expect(options.auto_merge_enabled).toBe(false);
    expect(options.mr_options?.find((o) => o.mr_iid === ARMED_IID)?.auto_fix_enabled).toBe(true);
    expect(options.mr_options?.find((o) => o.mr_iid === IDLE_IID)?.auto_fix_enabled).toBe(false);

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id, { expectedMrCount: 2 });

    const chip = session.mrStatusChip();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await expect(chip).toHaveAttribute("data-mr-count", "2", { timeout: 15_000 });
    const autoFixBadge = chip.getByTestId("mr-status-auto-fix-chip");
    await expect(autoFixBadge).toBeVisible();
    // The round comes from the armed MR, which has no fix rounds yet.
    await expect(autoFixBadge).toContainText("0/10");
    await expect(chip.getByTestId("mr-status-auto-merge-chip")).toBeVisible();
  });

  test("DOM order: pr-status-chip precedes mr-status-chip when a task has both an open PR and MR", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 407;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Dual chip MR");
    const task = await createTask(apiClient, seedData, "MR chip DOM order");
    await linkMR(apiClient, seedData, task.id, MR_IID);
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 501,
      pr_url: "https://github.com/test-org/test-repo/pull/501",
      pr_title: "Dual chip PR",
      head_branch: "feature/dual",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    const session = new SessionPage(testPage);
    await openTask(testPage, session, task.id);

    const statusBar = session.chatStatusBar();
    await expect(statusBar.getByTestId("pr-status-chip")).toBeVisible({ timeout: 15_000 });
    await expect(session.mrStatusChip()).toBeVisible({ timeout: 15_000 });

    const order = await statusBar.evaluate((el) => {
      const children = Array.from(
        el.querySelectorAll("[data-testid='pr-status-chip'], [data-testid='mr-status-chip']"),
      );
      return children.map((child) => child.getAttribute("data-testid"));
    });
    expect(order.indexOf("pr-status-chip")).toBeLessThan(order.indexOf("mr-status-chip"));
  });

  test("renders in the passthrough toolbar's status row for a passthrough session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const MR_IID = 408;
    await seedChipMR(apiClient, seedData.workspaceId, MR_IID, "Passthrough chip MR");
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
    });
    const profileId = await createPassthroughProfile(apiClient, "MR Chip Passthrough");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "MR chip passthrough row",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await linkMR(apiClient, seedData, task.id, MR_IID);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("MR chip passthrough row");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);

    const chip = session.mrStatusChipInPassthrough();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await expect(chip).toHaveAttribute("data-mr-iid", String(MR_IID));
  });
});

function mrSeed(iid: number, title: string) {
  const now = new Date().toISOString();
  return {
    id: iid + 10_000,
    iid,
    project_id: 101,
    title,
    url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`,
    state: "open",
    head_branch: `feature/${iid}`,
    head_sha: `sha-${iid}`,
    base_branch: "main",
    author_username: "contributor",
    project_namespace: "platform",
    project_path: GITLAB_PROJECT,
    body: "",
    draft: false,
    merge_status: "can_be_merged",
    has_conflicts: false,
    additions: 1,
    deletions: 1,
    reviewers: [],
    assignees: [],
    created_at: now,
    updated_at: now,
  };
}

function pipelineSeed(iid: number, status: string) {
  return {
    id: iid + 20_000,
    iid: 1,
    status,
    source: "push",
    ref: `feature/${iid}`,
    sha: `sha-${iid}`,
    web_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/pipelines/${iid + 20_000}`,
    jobs_total: 2,
    jobs_passing: status === "success" ? 2 : 0,
  };
}
