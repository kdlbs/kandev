import { execSync } from "node:child_process";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { KanbanPage } from "../../pages/kanban-page";

/**
 * Suite for per-watch cleanup_policy + manual cleanup sweep behavior
 * (Service.CleanupAllOrphanedReviewTasks, controller endpoint, settings
 * page button). Complements pr-watcher-merged-cleanup.spec.ts which covers
 * the per-watch poller-driven cleanup against the auto policy.
 */
test.describe("PR watcher cleanup policy", () => {
  /**
   * Auto-started review tasks used to be preserved forever because the
   * auto-start prompt itself counted as user activity. The fix tags those
   * messages with metadata.auto_start = true so they no longer block
   * deletion. This test exercises that regression: workflow with
   * auto_start_agent fires → PR merges → task IS deleted because no real
   * user message was authored.
   */
  test("auto-started task with no user message is deleted on merge", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Real cloned repo so the auto-start executor can check out the head branch.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "GitHub Test Repo",
      provider: "github",
      provider_owner: "testorg",
      provider_name: "testrepo",
    });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git branch -f feature/auto-only main", { cwd: repoDir, env: gitEnv });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 401,
        title: "Auto-start only",
        state: "open",
        head_branch: "feature/auto-only",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Workflow that auto-starts the review task on entry to "Working".
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Auto Cleanup Workflow");
    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt: 'e2e:message("agent ran")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      workflow.id,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    let prTask: { id: string; title: string } | undefined;
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          prTask = tasks.find((t) => t.title.includes("PR #401"));
          return prTask;
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn("PR #401: Auto-start only", inboxStep.id)).toBeVisible({
      timeout: 15_000,
    });

    // Run the agent (auto_start_agent on Working step) and let it finish.
    await apiClient.moveTask(prTask!.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("PR #401: Auto-start only", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Critical: do NOT inject a user message. The session exists, the agent
    // ran, but the only "user-authored" message is the auto-start prompt
    // tagged auto_start=true. That should NOT count as engagement.

    await apiClient.mockGitHubAddPRs([
      {
        number: 401,
        title: "Auto-start only",
        state: "closed",
        head_branch: "feature/auto-only",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);

    await expect(kanban.taskCardByTitle("PR #401: Auto-start only")).not.toBeVisible({
      timeout: 15_000,
    });
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.title.includes("PR #401"))).toBeUndefined();
  });

  /**
   * Tasks under cleanup_policy=never must survive merge regardless of
   * whether the user engaged. The settings dialog exposes this for users
   * who want to keep every task as a record.
   */
  test("cleanup_policy=never preserves task after PR is merged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 501,
        title: "Keep me forever",
        state: "open",
        head_branch: "feature/keep-me",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Keep Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      {
        repos: [{ owner: "testorg", name: "testrepo" }],
        cleanup_policy: "never",
      },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #501"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #501: Keep me forever")).toBeVisible({
      timeout: 15_000,
    });

    // Merge + cleanup should both no-op for this watch.
    await apiClient.mockGitHubAddPRs([
      {
        number: 501,
        title: "Keep me forever",
        state: "closed",
        head_branch: "feature/keep-me",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);
    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    const manualResult = await apiClient.cleanupMergedReviewTasks();
    expect(manualResult.deleted).toBe(0);

    await expect(kanban.taskCardByTitle("PR #501: Keep me forever")).toBeVisible({
      timeout: 5_000,
    });
    const { tasks: tasksAfter } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasksAfter.find((t) => t.title.includes("PR #501"))).toBeTruthy();
  });

  /**
   * cleanup_policy=always must delete the task even when the user authored
   * messages on it — the user opted into aggressive cleanup. Inverse of the
   * auto-policy "preserves started task" test.
   */
  test("cleanup_policy=always deletes task even with user messages", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "GitHub Test Repo",
      provider: "github",
      provider_owner: "testorg",
      provider_name: "testrepo",
    });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git branch -f feature/always main", { cwd: repoDir, env: gitEnv });

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 601,
        title: "Always delete",
        state: "open",
        head_branch: "feature/always",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Always Cleanup Workflow",
    );
    const inboxStep = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0);
    const workingStep = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
    await apiClient.updateWorkflowStep(workingStep.id, {
      prompt: 'e2e:message("ran")\n{{task_prompt}}',
      events: {
        on_enter: [{ type: "auto_start_agent" }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: doneStep.id } }],
      },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
    });

    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      workflow.id,
      inboxStep.id,
      seedData.agentProfileId,
      {
        repos: [{ owner: "testorg", name: "testrepo" }],
        cleanup_policy: "always",
      },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    let prTask: { id: string; title: string } | undefined;
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          prTask = tasks.find((t) => t.title.includes("PR #601"));
          return prTask;
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await apiClient.moveTask(prTask!.id, workflow.id, workingStep.id);
    await expect(kanban.taskCardInColumn("PR #601: Always delete", doneStep.id)).toBeVisible({
      timeout: 45_000,
    });

    // Inject a genuine user message — under auto-policy this would preserve
    // the task. Under always-policy it should NOT.
    const { sessions } = await apiClient.listTaskSessions(prTask!.id);
    expect(sessions.length).toBeGreaterThan(0);
    await apiClient.addUserMessage(prTask!.id, sessions[0].id, "wait, hold on");

    await apiClient.mockGitHubAddPRs([
      {
        number: 601,
        title: "Always delete",
        state: "closed",
        head_branch: "feature/always",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);

    await expect(kanban.taskCardByTitle("PR #601: Always delete")).not.toBeVisible({
      timeout: 15_000,
    });
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.title.includes("PR #601"))).toBeUndefined();
  });

  /**
   * Disabling a watch makes the per-watch poller skip it. Without the global
   * sweep its dedup rows would orphan forever. The manual cleanup endpoint
   * (and the periodic global sweep wired into the poller cycle) must still
   * reap merged tasks under that watch.
   */
  test("manual cleanup reaps merged tasks under disabled watch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 701,
        title: "Orphan via disabled watch",
        state: "open",
        head_branch: "feature/orphan",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Orphan Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #701"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #701: Orphan via disabled watch")).toBeVisible({
      timeout: 15_000,
    });

    // Disable the watch + merge the PR. The disabled watch is invisible to
    // the per-watch loop now, so without the global sweep the task survives.
    await apiClient.updateReviewWatch(watch.id, { enabled: false });
    await apiClient.mockGitHubAddPRs([
      {
        number: 701,
        title: "Orphan via disabled watch",
        state: "closed",
        head_branch: "feature/orphan",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const result = await apiClient.cleanupMergedReviewTasks();
    expect(result.deleted).toBe(1);

    await expect(kanban.taskCardByTitle("PR #701: Orphan via disabled watch")).not.toBeVisible({
      timeout: 15_000,
    });
  });

  /**
   * Deleting a watch should reap its tasks alongside the watch row. This is
   * the new service-level cascade (the store cascade alone would leave the
   * task rows behind with no dedup row pointing at them — strictly worse
   * than the pre-fix orphan since the global sweep can't see them either).
   */
  test("deleting a watch reaps its tasks via service cascade", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 801,
        title: "Cascade-delete me",
        state: "open",
        head_branch: "feature/cascade",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Cascade Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #801"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("PR #801: Cascade-delete me")).toBeVisible({
      timeout: 15_000,
    });

    // Delete the watch — service cascade should reap the task too.
    await apiClient.deleteReviewWatch(watch.id);

    await expect(kanban.taskCardByTitle("PR #801: Cascade-delete me")).not.toBeVisible({
      timeout: 15_000,
    });
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.title.includes("PR #801"))).toBeUndefined();
  });

  /**
   * UI smoke test for the settings-page "Clean up merged" button. Wires
   * through the same endpoint as cleanupMergedReviewTasks() but driven by
   * the frontend so we catch dialog/button regressions.
   */
  test("settings page cleanup button drains merged tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 901,
        title: "Drain me via UI",
        state: "open",
        head_branch: "feature/drain",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Drain Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id, watch.workspace_id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #901"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    // Merge the PR but don't trigger the watch — leave the cleanup to the
    // manual button so we know the click is what drove the deletion.
    await apiClient.mockGitHubAddPRs([
      {
        number: 901,
        title: "Drain me via UI",
        state: "closed",
        head_branch: "feature/drain",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    await testPage.goto("/settings/integrations/github");
    const cleanupButton = testPage.getByRole("button", { name: /clean up merged/i });
    await expect(cleanupButton).toBeVisible({ timeout: 10_000 });
    await cleanupButton.click();

    // Toast surfaces the deletion count.
    await expect(testPage.getByText(/deleted 1 task/i)).toBeVisible({ timeout: 10_000 });

    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.title.includes("PR #901"))).toBeUndefined();
  });
});
