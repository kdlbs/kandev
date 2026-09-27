// AC-COORDINATOR-NEEDS-YOU-001, -002, -003, -004: a real clarification
// question renders as a Decide-now Needs you item with working actions, and
// Queue groups tasks by their real PR aggregate state while the count strip
// stays in agreement with the lists (docs/specs/coordinator/requirements/needs-you.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { waitForHttp } from "../../helpers/causal-waits";
import { pollUntil } from "../../helpers/poll-until";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "../../../lib/coordinator/links";

const PR_OWNER = "coordinator-e2e-org";
const PR_REPO = "coordinator-e2e-repo";

test.describe("Coordinator Needs you and Queue", () => {
  test("a blocked clarification question renders as a Decide-now item with working actions (AC .002)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Planner",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    const title = "Needs You Golden Path";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the clarification task");

    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const sidebarRow = testPage.getByTestId(`sidebar-coordinator-${coordinator.id}`);
    await expect(sidebarRow).toBeVisible();
    await expect(sidebarRow).toContainText("Planner");

    await expect(testPage.getByTestId("count-needs-you")).toContainText("1");

    const card = testPage.getByTestId(`needs-you-item-${task.id}`);
    await expect(card).toBeVisible();
    await expect(card).toContainText(title);
    await expect(card.getByText("Decide now")).toBeVisible();
    await expect(card.getByText("The agent is waiting for your answer")).toBeVisible();
    await expect(card.getByText("Your answer, on the task")).toBeVisible();

    const askAboutThis = card.getByRole("button", { name: "Ask about this" });
    await expect(askAboutThis).toBeVisible();
    await expect(askAboutThis).toBeDisabled();

    await expect(card.getByRole("link", { name: "Open task" })).toBeVisible();
    // A question item is not a stall: no evidence popover trigger renders.
    await expect(card.getByRole("button", { name: "Show the evidence" })).toHaveCount(0);
  });

  test("Queue groups tasks by real PR aggregate state and the count strip agrees (AC .001, .004)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("coordinator-e2e");

    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Queue Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    const workingTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Working Task",
      seedData.agentProfileId,
      {
        description: "/sleep 60",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!workingTask.session_id) throw new Error("expected an active session for the /sleep task");
    await pollUntil(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(workingTask.id);
        return sessions.find((session) => session.id === workingTask.session_id)?.state ?? null;
      },
      (state) => state === "RUNNING" || state === "STARTING",
      30_000,
      "waiting for the /sleep session to be actively running for the Working group",
    );

    const readyTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Ready To Merge Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    // Approved review + passing checks + a clean merge state is the exact
    // combination `pullRequestAggregateState` (projector_pr.go) resolves to
    // "ready" (AC .001 rule 4).
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: readyTask.id,
      repository_id: seedData.repositoryId,
      owner: PR_OWNER,
      repo: PR_REPO,
      pr_number: 501,
      pr_url: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/501`,
      pr_title: "Ready to merge fixture",
      head_branch: "feature/ready",
      base_branch: "main",
      author_login: "coordinator-e2e",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      unresolved_review_threads: 0,
    });

    const inReviewTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "In Review Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    // Passing checks plus a pending required review is the exact combination
    // `pullRequestAwaitsReview` resolves to "awaiting_review" (AC .001 rule 5).
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: inReviewTask.id,
      repository_id: seedData.repositoryId,
      owner: PR_OWNER,
      repo: PR_REPO,
      pr_number: 502,
      pr_url: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/502`,
      pr_title: "In review fixture",
      head_branch: "feature/in-review",
      base_branch: "main",
      author_login: "coordinator-e2e",
      state: "open",
      review_state: "pending",
      checks_state: "success",
      mergeable_state: "clean",
      required_reviews: 1,
      pending_review_count: 1,
      // A nonzero unresolved-thread count would win the backend's precedence
      // order as "failure" before "awaiting_review" is ever considered
      // (pullRequestAggregateState, projector_pr.go): the pending required
      // review must be the only attention signal for this fixture.
      unresolved_review_threads: 0,
    });

    await pollUntil(
      async () =>
        Promise.all([apiClient.listTaskPRs(readyTask.id), apiClient.listTaskPRs(inReviewTask.id)]),
      ([readyPRs, inReviewPRs]) => readyPRs.length === 1 && inReviewPRs.length === 1,
      15_000,
      "waiting for both seeded PR associations to read back",
    );

    const prsPromise = waitForHttp(testPage, "GET", /\/api\/v1\/github\/task-prs$/);
    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    await prsPromise;

    await expect(testPage.getByTestId("count-working")).toContainText("1");
    await expect(testPage.getByTestId("count-in-review")).toContainText("1");
    await expect(testPage.getByTestId("count-ready-to-merge")).toContainText("1");

    await expect(testPage.getByTestId("queue-group-working")).toBeVisible();
    const workingRow = testPage.getByTestId(`queue-row-${workingTask.id}`);
    await expect(workingRow).toBeVisible();
    await expect(workingRow).toContainText(/Running|Starting/);

    await expect(testPage.getByTestId("queue-group-ready_to_merge")).toBeVisible();
    const readyRow = testPage.getByTestId(`queue-row-${readyTask.id}`);
    await expect(readyRow).toBeVisible();
    await expect(readyRow).toContainText("open");

    await expect(testPage.getByTestId("queue-group-in_review")).toBeVisible();
    const inReviewRow = testPage.getByTestId(`queue-row-${inReviewTask.id}`);
    await expect(inReviewRow).toBeVisible();
    await expect(inReviewRow).toContainText("0 unresolved threads");

    // Opening the task is a Queue row's only action (AC .004.5): it is a
    // plain link, not a button with a menu.
    await expect(readyRow).toHaveAttribute("href", new RegExp(readyTask.id));
  });
});
