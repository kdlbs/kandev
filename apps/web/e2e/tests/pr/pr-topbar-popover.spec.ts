import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 42;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;

type SeedResult = {
  workflowId: string;
  inboxStepId: string;
  workingStepId: string;
  doneStepId: string;
  taskId: string;
};

/**
 * Stand up a workspace + workflow + task that reaches the Done column
 * immediately (auto-start + on_turn_complete moves it). Returns the IDs the
 * spec needs to seed PR data + open the task.
 */
async function seedTask(
  apiClient: ApiClient,
  workspaceId: string,
  agentProfileId: string,
  repositoryId: string,
  title: string,
): Promise<SeedResult> {
  const workflow = await apiClient.createWorkflow(workspaceId, `${title} Workflow`);
  const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
  const working = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

  await apiClient.updateWorkflowStep(working.id, {
    prompt: 'e2e:message("done")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
    },
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");

  const task = await apiClient.createTask(workspaceId, title, {
    workflow_id: workflow.id,
    workflow_step_id: inbox.id,
    agent_profile_id: agentProfileId,
    repository_ids: [repositoryId],
  });

  return {
    workflowId: workflow.id,
    inboxStepId: inbox.id,
    workingStepId: working.id,
    doneStepId: done.id,
    taskId: task.id,
  };
}

async function associatePR(
  apiClient: ApiClient,
  taskId: string,
  overrides:
    | Parameters<ApiClient["mockGitHubAssociateTaskPR"]>[0]
    | Partial<Parameters<ApiClient["mockGitHubAssociateTaskPR"]>[0]> = {},
) {
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: OWNER,
    repo: REPO,
    pr_number: PR_NUMBER,
    pr_url: PR_URL,
    pr_title: "Add CI popover",
    head_branch: "feat/popover",
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    additions: 100,
    deletions: 5,
    ...overrides,
  });
}

async function openTaskAndWait(
  testPage: import("@playwright/test").Page,
  apiClient: ApiClient,
  seed: SeedResult,
  title: string,
): Promise<SessionPage> {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await apiClient.moveTask(seed.taskId, seed.workflowId, seed.workingStepId);
  await expect(kanban.taskCardInColumn(title, seed.doneStepId)).toBeVisible({ timeout: 45_000 });
  await kanban.taskCardInColumn(title, seed.doneStepId).click();
  await expect(testPage).toHaveURL(/\/[st]\//, { timeout: 15_000 });
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
  return session;
}

test.describe("PR top-bar CI popover", () => {
  test("counts row uses TaskPR aggregates and hides zero-count buckets", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Counts Aggregates";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 27,
      checks_passing: 22,
      review_state: "approved",
      review_count: 1,
      pending_review_count: 0,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();

    await expect(session.prCheckGroupCount("passed")).toHaveText("22");
    await expect(session.prCheckGroupCount("failed")).toHaveText("5");
    // No in-progress group when checks_state=failure with full passing count.
    await expect(session.prCheckGroup("in_progress")).toHaveCount(0);
  });

  test("shows only Passed when all checks succeed (in_progress + failed hidden)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "All Passed";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "success",
      checks_total: 22,
      checks_passing: 22,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();

    await expect(session.prCheckGroup("passed")).toBeVisible();
    await expect(session.prCheckGroupCount("passed")).toHaveText("22");
    await expect(session.prCheckGroup("in_progress")).toHaveCount(0);
    await expect(session.prCheckGroup("failed")).toHaveCount(0);
    // Passed group is header-only — no workflow rows beneath it.
    await expect(session.prCheckGroup("passed").getByTestId("pr-workflow-row")).toHaveCount(0);
  });

  test("workflow rollup groups jobs and shows (N/M passed) badge under Failed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Workflow Rollup";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 6,
      checks_passing: 4,
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      checks: [
        { name: "Lint / check", status: "completed", conclusion: "failure", html_url: "" },
        { name: "E2E / a", status: "completed", conclusion: "failure", html_url: "" },
        { name: "E2E / b", status: "completed", conclusion: "success", html_url: "" },
        { name: "E2E / c", status: "completed", conclusion: "success", html_url: "" },
        { name: "E2E / d", status: "completed", conclusion: "success", html_url: "" },
        { name: "E2E / e", status: "completed", conclusion: "success", html_url: "" },
      ],
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();

    await expect(session.prWorkflowRow("Lint")).toBeVisible({ timeout: 10_000 });
    await expect(session.prWorkflowRow("E2E")).toBeVisible();
    await expect(session.prWorkflowRow("Lint")).toContainText("0/1 passed");
    await expect(session.prWorkflowRow("E2E")).toContainText("4/5 passed");
  });

  test("review row shows N / M required + unresolved comments count", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Review + Comments";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      review_state: "approved",
      review_count: 1,
      pending_review_count: 0,
      required_reviews: 2,
      unresolved_review_threads: 3,
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();

    await expect(session.prReviewRow()).toBeVisible();
    await expect(session.prReviewRow()).toContainText("Approved");
    await expect(session.prReviewRow()).toContainText("1 / 2");
    await expect(session.prCommentsRow()).toBeVisible();
    await expect(session.prCommentsRow()).toContainText("3 unresolved comments");
  });

  test("comments row hidden when unresolved_review_threads = 0", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "No Unresolved";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      review_state: "approved",
      review_count: 1,
      unresolved_review_threads: 0,
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();

    await expect(session.prReviewRow()).toBeVisible();
    await expect(session.prCommentsRow()).toHaveCount(0);
  });

  test("hover opens popover; mouse-leave closes it after the close delay", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Hover Open Close";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);

    await session.hoverPRTopbar();
    // Move the cursor far away from the popover; close timer fires and the
    // popover unmounts.
    await testPage.mouse.move(0, 0);
    await expect(session.prTopbarPopover()).toHaveCount(0, { timeout: 5_000 });
  });

  test("failed workflow row exposes open + add-to-context buttons", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Failed Affordances";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 1,
      checks_passing: 0,
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      checks: [
        {
          name: "Lint / check",
          status: "completed",
          conclusion: "failure",
          html_url: "https://example.com/lint-run-1",
          output: "lint failed",
        },
      ],
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();
    // Move into the popover so it stays open while we read the row.
    await session.prTopbarPopover().hover();

    await expect(session.prWorkflowRow("Lint")).toBeVisible({ timeout: 10_000 });
    await expect(session.prWorkflowOpenButton("Lint")).toBeVisible();
    await expect(session.prWorkflowAddContextButton("Lint")).toBeVisible();
  });

  test("header external link points at the PR's GitHub /checks tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "External Link";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "success",
      checks_total: 1,
      checks_passing: 1,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();
    await expect(session.prPopoverExternalLink()).toHaveAttribute("href", `${PR_URL}/checks`);
  });

  test("empty state when PR has no checks at all", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const title = "No Checks Yet";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "",
      checks_total: 0,
      checks_passing: 0,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();
    await expect(session.prChecksEmpty()).toBeVisible();
    await expect(session.prChecksEmpty()).toContainText("No checks have started");
  });

  test("auth lost surfaces a Reconnect GitHub link", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const title = "Reconnect GitHub";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await apiClient.mockGitHubSetAuthHealth({
      authenticated: false,
      error: "token expired",
    });
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 2,
      checks_passing: 1,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();
    await expect(session.prPopoverReconnectLink()).toBeVisible();
    await expect(session.prPopoverReconnectLink()).toHaveAttribute("href", /settings/);
  });

  test("counts update on a re-associate (live refresh)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Live Refresh";
    const seed = await seedTask(
      apiClient,
      seedData.workspaceId,
      seedData.agentProfileId,
      seedData.repositoryId,
      title,
    );
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 27,
      checks_passing: 22,
    });
    const session = await openTaskAndWait(testPage, apiClient, seed, title);
    await session.hoverPRTopbar();
    await session.prTopbarPopover().hover();
    await expect(session.prCheckGroupCount("passed")).toHaveText("22");

    // Re-POST with a new passing count to simulate a CI tick.
    await associatePR(apiClient, seed.taskId, {
      checks_state: "failure",
      checks_total: 27,
      checks_passing: 25,
    });
    await expect(session.prCheckGroupCount("passed")).toHaveText("25", { timeout: 5_000 });
  });
});
