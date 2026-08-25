import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

/**
 * E2E coverage for the github_pr_merged ("Pull request merged") trigger type.
 *
 * Spec: docs/specs/office/requirements/automations-pr-merged-trigger.md
 * Decisions pinned here:
 *  - Picker shows "Pull request merged" under GitHub group, immediately after "New pull requests"
 *  - Config round-trip: all_repos, repos, base_branches survive save/reopen
 *  - {} config → all_repos=false, dead-config warning shown
 *  - Checking "All repositories" CLEARS repos; unchecking leaves list as-is
 *  - Toggle cycle: check → save → reopen → uncheck → empty repos + warning
 *  - workspaceId pass-through asserted on the outbound /api/v1/github/orgs request
 *  - List page badge label "GitHub PR Merged" in purple
 *  - Info tooltip mentions workspace and up-to-a-minute detection lag
 *  - Repository picker is ENABLED (unlike github_pr which disables it)
 */
test.describe("automations — Pull request merged trigger", () => {
  test("picker shows 'Pull request merged' under GitHub group and is selectable", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.addConditionButton.click();

    // "Pull request merged" should appear
    await expect(testPage.getByRole("option", { name: /Pull request merged/i })).toBeVisible({
      timeout: 5_000,
    });

    // It should be under the GitHub group (the GitHub heading is visible)
    await expect(testPage.getByRole("group", { name: /GitHub/i })).toBeVisible();

    // Select it
    await testPage.getByRole("option", { name: /Pull request merged/i }).click();

    // A trigger card should appear with the summary text
    await expect(testPage.getByText(/PR merged/i).first()).toBeVisible({
      timeout: 5_000,
    });
  });

  test("'Pull request merged' appears immediately after 'New pull requests' in picker", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.addConditionButton.click();
    await expect(testPage.getByRole("option", { name: /Pull request merged/i })).toBeVisible({
      timeout: 5_000,
    });

    // Collect all option labels in the GitHub group (in DOM order)
    const githubGroup = testPage.getByRole("group", { name: /GitHub/i });
    const options = githubGroup.getByRole("option");
    const labels = await options.allTextContents();

    const prIdx = labels.findIndex((l) => /New pull requests/i.test(l));
    const mergedIdx = labels.findIndex((l) => /Pull request merged/i.test(l));

    expect(prIdx).toBeGreaterThanOrEqual(0);
    expect(mergedIdx).toBe(prIdx + 1);
  });

  test("config round-trips: all_repos, repos and base_branches survive save/reopen", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.nameInput.fill("PR Merged Round-trip");
    await automations.selectWorkflow("E2E Workflow");

    // Add the condition
    await automations.addConditionButton.click();
    await testPage.getByRole("option", { name: /Pull request merged/i }).click();

    // Expand the trigger card by clicking the summary button
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // The panel should show "All repositories" checked by default (all_repos:true from registry default)
    const triggerCard = testPage.getByTestId("trigger-card-github_pr_merged");
    const allReposSwitch = triggerCard.getByRole("switch", {
      name: /All repositories allowed/i,
    });
    await expect(allReposSwitch).toBeChecked({ timeout: 5_000 });

    // Uncheck "All repositories"
    await allReposSwitch.click();
    await expect(allReposSwitch).not.toBeChecked();

    // Dead-config warning appears (repos is now empty)
    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible();

    // Fill in a base branch
    const branchInput = testPage.getByPlaceholder(/main, release/i);
    await branchInput.fill("main");
    await branchInput.blur();

    // Save
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });

    // Reopen
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.openByName("PR Merged Round-trip");
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();

    // Expand trigger card again
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // all_repos should still be unchecked
    await expect(
      testPage.getByRole("switch", { name: /All repositories allowed/i }),
    ).not.toBeChecked({ timeout: 5_000 });

    // Dead-config warning should still be visible (no repos selected)
    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible();
  });

  test("stored {} config renders with all_repos unchecked and dead-config warning", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Empty Config Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });

    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: {},
      enabled: true,
    });

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });
    // Expand the trigger card
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // all_repos should be unchecked (absent-field default is false, not true)
    await expect(
      testPage.getByRole("switch", { name: /All repositories allowed/i }),
    ).not.toBeChecked({ timeout: 5_000 });

    // Dead-config warning must be shown
    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible({ timeout: 5_000 });
  });

  test("info tooltip mentions detection lag", async ({ testPage, seedData, apiClient }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Tooltip Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    // Hover the info icon on the trigger card to reveal tooltip
    await testPage.getByTestId("trigger-info-icon").hover();

    // The tooltip should contain "minute" (detection lag) — per triggerInfoGithubPrMerged i18n key
    const tooltip = testPage.getByTestId("trigger-info-tooltip");
    await expect(tooltip).toBeVisible({ timeout: 5_000 });
    await expect(tooltip).toContainText(/minute/i);
  });

  test("workspaceId is passed to the orgs request when the config panel mounts", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.addConditionButton.click();
    await testPage.getByRole("option", { name: /Pull request merged/i }).click();

    // Interceptor must be set up BEFORE expanding — the orgs fetch fires on
    // RepoFilterSelector mount, which happens when the config panel becomes visible.
    const orgRequestPromise = testPage.waitForRequest(
      (req) =>
        req.url().includes("/api/v1/github/orgs") &&
        new URL(req.url()).searchParams.get("workspace_id") === seedData.workspaceId,
      { timeout: 10_000 },
    );

    // Expand the trigger card — this mounts RepoFilterSelector, which fires the orgs fetch
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // The key assertion: the outbound request carries this workspace's id
    const orgRequest = await orgRequestPromise;
    const url = new URL(orgRequest.url());
    expect(url.searchParams.get("workspace_id")).toBe(seedData.workspaceId);
  });

  test("checking 'All repositories' clears repos; toggle cycle produces warning on reopen + uncheck", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Seed an automation with a non-empty repos list and all_repos: false
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Toggle Cycle Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: {
        all_repos: false,
        repos: [{ owner: "acme", name: "api" }],
        base_branches: [],
      },
      enabled: true,
    });

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });
    // Expand the trigger card
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // all_repos is false, no dead-config warning (repos has one entry)
    await expect(
      testPage.getByRole("switch", { name: /All repositories allowed/i }),
    ).not.toBeChecked({ timeout: 5_000 });
    await expect(testPage.getByText(/No repositories selected/i)).not.toBeVisible();

    // CHECK "All repositories" → should clear repos
    const allReposSwitch = testPage.getByRole("switch", {
      name: /All repositories allowed/i,
    });
    await allReposSwitch.click();
    await expect(allReposSwitch).toBeChecked();
    // Wait for the parent draft to render the cleared repository list before
    // submitting. The switch updates first; saving in that intermediate render
    // can otherwise race the config state update when the suite runs in full.
    await expect(testPage.getByText("acme/api", { exact: true })).not.toBeVisible({
      timeout: 5_000,
    });

    const automationsPage = new AutomationsPage(testPage, seedData.workspaceId);

    // Save — updating a pre-existing automation stays on the editor page (no redirect)
    await expect(automationsPage.saveButton).toBeEnabled({ timeout: 5_000 });
    await automationsPage.saveButton.click();

    // Wait for the shared save coordinator to finish. The button's accessible
    // name changes from "Save changes" to "Saving changes" while the request
    // is in flight, so waiting for that name to disappear can reload the page
    // before trigger persistence completes.
    await expect(testPage.getByTestId("settings-floating-save")).toHaveAttribute(
      "data-status",
      "saved",
      { timeout: 10_000 },
    );

    // Reload the page to verify the saved state is persisted
    await testPage.reload();
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    // Expand trigger card again
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // all_repos is now true
    const checkboxAfterSave = testPage.getByRole("switch", {
      name: /All repositories allowed/i,
    });
    await expect(checkboxAfterSave).toBeChecked({ timeout: 5_000 });

    // UNCHECK "All repositories" → repos was cleared, so dead-config warning appears
    await checkboxAfterSave.click();
    await expect(checkboxAfterSave).not.toBeChecked();

    // repos is empty (was cleared when "All repositories" was checked), so warning shows
    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible({ timeout: 5_000 });
  });

  test("list page badge shows 'GitHub PR Merged' label", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Badge Label Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    // The badge for this automation should show "GitHub PR Merged"
    const row = automations.table.locator("tr", { hasText: "Badge Label Test" });
    await expect(row.getByText("GitHub PR Merged")).toBeVisible({ timeout: 10_000 });
  });

  test("repository picker is enabled for github_pr_merged (unlike github_pr)", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Picker Enabled Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    // Expand the trigger card
    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .click();

    // Uncheck "All repositories" to show the repo selector
    const triggerCard = testPage.getByTestId("trigger-card-github_pr_merged");
    const allReposSwitch = triggerCard.getByRole("switch", {
      name: /All repositories allowed/i,
    });
    await expect(allReposSwitch).toBeChecked({ timeout: 5_000 });
    await allReposSwitch.click();

    // The "Add repository" button appears when all_repos=false and must NOT be disabled.
    // (For github_pr the picker is disabled; for github_pr_merged it must be enabled.)
    const addRepoButton = triggerCard.getByRole("button", { name: /Add repository/i });
    await expect(addRepoButton).toBeVisible({ timeout: 5_000 });
    await expect(addRepoButton).not.toBeDisabled();
  });

  // ---------------------------------------------------------------------------
  // Agent behavior — spec lines 1487–1503
  // These use the mock agent's inline script harness (e2e:mcp:... directives),
  // NOT /e2e:<name> scenarios, because the inline script receives the already-
  // interpolated {{data.task_id}} value whereas scenario handlers do not.
  // ---------------------------------------------------------------------------

  test("agent behavior: scripted agent archives the correct task id from trigger data", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createTask(seedData.workspaceId, "PR Merged Archive Target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Archive on PR Merge",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt:
        'e2e:mcp:kandev:archive_task_kandev({"task_id":"{{data.task_id}}"})\ne2e:message("archived-done")',
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const { run_task_id } = await apiClient.firePRMerged({
      taskId: target.id,
      automationId: automation.id,
      owner: "acme",
      repo: "api",
    });

    await testPage.goto(`/t/${run_task_id}`);
    await expect(testPage.getByText("archived-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Target task should no longer appear in the active task list
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === target.id)).toBeUndefined();
  });

  test("agent behavior: a wrong same-owner archive target is rejected", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createTask(seedData.workspaceId, "Bound Archive Target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const decoy = await apiClient.createTask(seedData.workspaceId, "Same Owner Decoy", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Reject Wrong Archive Target",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt: `e2e:mcp:kandev:archive_task_kandev({"task_id":"${decoy.id}"})\ne2e:message("wrong-target-rejected")`,
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const { run_task_id } = await apiClient.firePRMerged({
      taskId: target.id,
      automationId: automation.id,
      owner: "acme",
      repo: "api",
    });

    await testPage.goto(`/t/${run_task_id}`);
    await expect(testPage.getByText("wrong-target-rejected", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((task) => task.id === target.id)).toBeDefined();
    expect(tasks.find((task) => task.id === decoy.id)).toBeDefined();
  });

  test("agent behavior: archive_task_kandev on an already-archived target succeeds and run completes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createTask(seedData.workspaceId, "Pre-archived Target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(target.id);

    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Archive Already Archived",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt:
        'e2e:mcp:kandev:archive_task_kandev({"task_id":"{{data.task_id}}"})\ne2e:message("already-archived-done")',
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const { run_task_id } = await apiClient.firePRMerged({
      taskId: target.id,
      automationId: automation.id,
      owner: "acme",
      repo: "api",
    });

    // Run must complete — the marker appearing proves the turn ended cleanly
    await testPage.goto(`/t/${run_task_id}`);
    await expect(testPage.getByText("already-archived-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("agent behavior: run succeeds even when archive_task_kandev fails for a deleted target", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createTask(seedData.workspaceId, "Soon-deleted Target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Archive Deleted Target",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      // e2e:delay gives the test time to delete the target before the archive call
      prompt:
        'e2e:delay(1000)\ne2e:mcp:kandev:archive_task_kandev({"task_id":"{{data.task_id}}"})\ne2e:message("deleted-done")',
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    // Wait until the event has created the automation run, then delete the
    // source task while the delay keeps the agent before its archive call.
    const { run_task_id } = await apiClient.firePRMerged({
      taskId: target.id,
      automationId: automation.id,
      owner: "acme",
      repo: "api",
    });
    await apiClient.deleteTask(target.id);

    // Run must still complete cleanly — MCP error on a deleted task must not crash the turn
    await testPage.goto(`/t/${run_task_id}`);
    await expect(testPage.getByText("deleted-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });

  test("agent behavior: run succeeds and target is unarchived when agent makes no archive call", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = await apiClient.createTask(seedData.workspaceId, "Should Stay Unarchived", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "No Archive Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt: 'e2e:message("no-archive-done")',
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const { run_task_id } = await apiClient.firePRMerged({
      taskId: target.id,
      automationId: automation.id,
      owner: "acme",
      repo: "api",
    });

    await testPage.goto(`/t/${run_task_id}`);
    await expect(testPage.getByText("no-archive-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    // Target must still appear in the active task list — no archive was called
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.find((t) => t.id === target.id)).toBeDefined();
  });

  test("agent behavior: manual run fires with no task_id and does not archive any task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Manual Trigger Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt: 'e2e:message("manual-done")',
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr_merged",
      config: { all_repos: true, repos: [], base_branches: [] },
      enabled: true,
    });

    const result = await apiClient.triggerAutomationManual(automation.id);
    expect(result.run_task_id).toBeTruthy();

    await testPage.goto(`/t/${result.run_task_id!}`);
    await expect(testPage.getByText("manual-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });
});
