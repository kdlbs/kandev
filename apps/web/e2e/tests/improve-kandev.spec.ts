import type { Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";

/**
 * E2E tests for the Improve Kandev dialog. The bootstrap endpoint is mocked
 * because it would otherwise clone https://github.com/kdlbs/kandev and shell
 * out to the gh CLI. The system/health endpoint is mocked to keep the intro
 * screen out of the GhAuthMissing branch regardless of backend state.
 */

const BOOTSTRAP_URL = "**/api/v1/system/improve-kandev/bootstrap";
const HEALTH_URL = "**/api/v1/system/health";

type ForkStatus = "writable" | "ready" | "blocked_emu" | "unknown";

type BootstrapOverrides = {
  /** Override the dedicated workspace the response points at. */
  workspaceId?: string;
  /** Override the repository id (must exist in the workspace's repo list). */
  repositoryId?: string;
  /** Override the workflow id (must belong to the workspace). */
  workflowId?: string;
  github_login?: string;
  has_write_access?: boolean;
  fork_status?: ForkStatus;
  fork_message?: string;
  issueWorkflowId?: string;
  /**
   * When provided, the bootstrap route handler awaits this promise before
   * fulfilling the response. Tests use it to keep the dialog in its
   * `loading` state long enough to assert the disabled submit UI.
   */
  bootstrapHold?: Promise<void>;
};

async function mockImproveKandevApis(
  page: Page,
  seed: { workspaceId: string; repositoryId: string; workflowId: string },
  overrides: BootstrapOverrides = {},
): Promise<void> {
  const bundleDir = "/tmp/kandev-improve-e2e";
  const hasWrite = overrides.has_write_access ?? false;
  // Default fork_status mirrors how the backend would respond for the given
  // write-access value: writable when the user has push access, otherwise
  // unknown (the safe fall-through that lets the dialog proceed normally).
  const forkStatus: ForkStatus = overrides.fork_status ?? (hasWrite ? "writable" : "unknown");

  await page.route(HEALTH_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ healthy: true, issues: [] }),
    }),
  );

  await page.route(BOOTSTRAP_URL, async (route) => {
    if (overrides.bootstrapHold) {
      await overrides.bootstrapHold;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        workspace_id: overrides.workspaceId ?? seed.workspaceId,
        repository_id: overrides.repositoryId ?? seed.repositoryId,
        workflow_id: overrides.workflowId ?? seed.workflowId,
        issue_workflow_id: overrides.issueWorkflowId ?? seed.workflowId,
        branch: "main",
        bundle_dir: bundleDir,
        bundle_file: `${bundleDir}/diagnostic-bundle.zip`,
        github_login: overrides.github_login ?? "octocat",
        has_write_access: hasWrite,
        fork_status: forkStatus,
        ...(overrides.fork_message ? { fork_message: overrides.fork_message } : {}),
      }),
    });
  });
}

test.describe("Improve Kandev dialog", () => {
  test("dismissed intro is persisted and later opens the create dialog directly", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    await mockImproveKandevApis(testPage, seedData);
    await testPage.goto("/");

    await testPage.getByTestId("sidebar-improve-kandev-button").click();
    const introDialog = testPage.getByRole("dialog", { name: "Improve Kandev" });
    const dismissPreference = introDialog.getByTestId("improve-kandev-skip-intro");
    await expect(dismissPreference).toBeVisible();
    await dismissPreference.click();
    await expect
      .poll(() =>
        testPage.evaluate(() => window.localStorage.getItem("kandev.improveKandev.skipIntro")),
      )
      .toBe("true");

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible({ timeout: 10_000 });

    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("create-task-dialog")).toBeHidden();
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Preparing kandev repository…")).toHaveCount(0);
    await expect(testPage.getByText(/Kandev is open source/)).toHaveCount(0);
  });

  test("Open issue creates a task in the report-only workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const issueWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Report Kandev issue",
      "simple",
    );
    const issueSteps = await apiClient.listWorkflowSteps(issueWorkflow.id);
    const issueStartStep =
      issueSteps.steps.find((step) => step.is_start_step) ?? issueSteps.steps[0];
    await apiClient.createWorkspace("Improve Kandev");
    // The upstream create dialog hides the title field when auto-generated
    // task titles are enabled; disable them so the tests can type titles.
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    await mockImproveKandevApis(testPage, seedData, { issueWorkflowId: issueWorkflow.id });
    await testPage.goto("/");
    await testPage.getByTestId("sidebar-improve-kandev-button").click();
    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    const issueTab = createDialog.getByRole("tab", { name: "Open issue" });
    await expect(issueTab).toBeVisible();
    await issueTab.click();
    await expect(
      createDialog.getByText(/only create a GitHub issue.*will not implement/i),
    ).toBeVisible();

    const title = "Document task ownership in parallel sessions";
    await createDialog.getByTestId("task-title-input").fill(title);
    await createDialog
      .getByTestId("task-description-input")
      .fill("Users cannot tell which task owns a worktree when several agents run.");
    const submit = createDialog.getByTestId("submit-start-agent");
    await expect(submit).toBeEnabled({ timeout: 10_000 });
    await submit.click();
    await expect(createDialog).toBeHidden({ timeout: 10_000 });

    await expect
      .poll(async () => (await apiClient.listTasks(seedData.workspaceId)).tasks)
      .toContainEqual(
        expect.objectContaining({
          title,
          workflow_step_id: issueStartStep.id,
        }),
      );
  });

  test("improve task lands in the dedicated Improve Kandev workspace", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The dedicated workspace is configuration-immutable: workflows and
    // repositories cannot be created in it via the API. Seed them under a
    // temporary name, then rename the workspace to "Improve Kandev" so the
    // mocked bootstrap's workspace_id exists in the backend and the dialog
    // lists its repositories and creates the task there.
    const staging = await apiClient.createWorkspace("Improve Kandev Setup");
    const dedicatedWorkflow = await apiClient.createWorkflow(
      staging.id,
      "Improve Kandev",
      "simple",
    );
    const dedicatedRepo = await apiClient.createRepository(
      staging.id,
      seedData.repositoryPath,
      "main",
    );
    await apiClient.updateWorkspace(staging.id, { name: "Improve Kandev" });
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    const dedicated = staging;
    await mockImproveKandevApis(testPage, seedData, {
      workspaceId: dedicated.id,
      workflowId: dedicatedWorkflow.id,
      issueWorkflowId: dedicatedWorkflow.id,
      repositoryId: dedicatedRepo.id,
    });

    await testPage.goto("/");
    await testPage.getByTestId("sidebar-improve-kandev-button").click();
    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });

    const title = "Isolate improve tasks in their own workspace";
    await createDialog.getByTestId("task-title-input").fill(title);
    await createDialog
      .getByTestId("task-description-input")
      .fill("Improve Kandev tasks must not mix with regular work.");
    const submit = createDialog.getByTestId("submit-start-agent");
    await expect(submit).toBeEnabled({ timeout: 10_000 });
    await submit.click();
    await expect(createDialog).toBeHidden({ timeout: 10_000 });

    // The task lands in the dedicated workspace…
    await expect
      .poll(async () => (await apiClient.listTasks(dedicated.id)).tasks)
      .toContainEqual(expect.objectContaining({ title }));
    // …and never in the user's active workspace.
    await expect
      .poll(async () => (await apiClient.listTasks(seedData.workspaceId)).tasks)
      .not.toContainEqual(expect.objectContaining({ title }));
  });

  test("New task button opens the Improve Kandev dialog in the dedicated workspace", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const dedicated = await apiClient.createWorkspace("Improve Kandev");
    // Mock the health/bootstrap APIs so the dialog intro is reachable without
    // a real GitHub connection (same as the other tests in this file).
    await mockImproveKandevApis(testPage, seedData, {
      workspaceId: dedicated.id,
    });

    await testPage.goto("/");
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${dedicated.id}`).click();
    await expect(testPage.getByTestId("kanban-board")).toBeVisible();
    await expect(testPage.getByTestId("sidebar-workspace-trigger")).toHaveText(/Improve Kandev/);

    await testPage.getByTestId("create-task-button").click();

    // Inside the dedicated workspace the New Task entry opens the Improve
    // Kandev dialog — never the generic create-task dialog.
    await expect(testPage.getByRole("dialog", { name: "Improve Kandev" })).toBeVisible();
    await expect(testPage.getByTestId("improve-kandev-proceed")).toBeEnabled();
    await expect(testPage.getByTestId("create-task-dialog")).toHaveCount(0);
  });

  test("intro → create flow shows workflow preview, useful info, and fork banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "octocat",
      has_write_access: false,
    });

    await testPage.goto("/");

    // Post-overhaul: the Improve Kandev opener moved to the AppSidebar footer.
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    // Intro screen
    const introDialog = testPage.getByRole("dialog", { name: "Improve Kandev" });
    await expect(introDialog).toBeVisible();
    await expect(introDialog.getByText(/Kandev is open source/)).toBeVisible();
    await expect(introDialog.getByText(/forks .* to your GitHub account/)).toBeVisible();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    // Create dialog mounts after Contribute is clicked
    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });

    // Bug fix / Feature request kind tabs
    await expect(createDialog.getByRole("tab", { name: "Bug fix" })).toBeVisible();
    await expect(createDialog.getByRole("tab", { name: "Feature request" })).toBeVisible();

    // Contributor banner (fork mode)
    await expect(createDialog.getByText("@octocat")).toBeVisible();
    await expect(
      createDialog.getByText(/agent will fork kdlbs\/kandev to your account/i),
    ).toBeVisible();

    // Workflow preview header
    await expect(createDialog.getByText("Workflow", { exact: true })).toBeVisible();

    // Useful info collapsible expands and lists shell + skill commands
    const usefulInfoTrigger = createDialog.getByRole("button", { name: /useful commands/i });
    await expect(usefulInfoTrigger).toBeVisible();
    await usefulInfoTrigger.click();
    await expect(createDialog.getByText("make install && make dev")).toBeVisible();
    await expect(createDialog.getByText("/commit", { exact: true })).toBeVisible();
    await expect(createDialog.getByText("/pr-fixup", { exact: true })).toBeVisible();

    // Remote-tab toggle is hidden because the repository is locked to kandev
    await expect(createDialog.getByTestId("source-mode-remote")).toHaveCount(0);
  });

  test("contributor banner shows direct-push copy when user has write access", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "kandev-maint",
      has_write_access: true,
    });

    await testPage.goto("/");
    // Post-overhaul: the Improve Kandev opener moved to the AppSidebar footer.
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    await expect(createDialog.getByText("@kandev-maint")).toBeVisible();
    await expect(
      createDialog.getByText(/push directly to a branch on the upstream repo/i),
    ).toBeVisible();
  });

  test("blocks implementation but allows issue reporting for an Enterprise Managed User", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    const blockedMessage =
      "Your GitHub account appears to be an Enterprise Managed User (EMU) account, " +
      "which typically cannot fork repositories outside your owning enterprise. " +
      "The PR step would fail when forking kdlbs/kandev. Contact your GitHub admin " +
      "if you'd like to enable this, or contribute via another account.";
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "alice_corp",
      has_write_access: false,
      fork_status: "blocked_emu",
      fork_message: blockedMessage,
    });

    await testPage.goto("/");
    // Post-overhaul: the Improve Kandev opener moved to the AppSidebar footer.
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    await expect(createDialog.getByText(blockedMessage)).toBeVisible();
    await createDialog.getByTestId("task-title-input").fill("EMU contribution");
    await createDialog.getByTestId("task-description-input").fill("Describe the problem");
    await expect(createDialog.getByTestId("submit-start-agent")).toBeDisabled();

    await createDialog.getByRole("tab", { name: "Open issue" }).click();
    await expect(createDialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 10_000 });
  });

  test("locked mode hides workflow picker and source-mode switch (URL / None)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    // Add a second workflow to the seeded workspace so the create dialog
    // would normally render the workflow selector (workflows.length > 1).
    // The locked-fields path in WorkflowSection must still suppress it.
    await apiClient.createWorkflow(seedData.workspaceId, "Extra Workflow", "simple");

    await mockImproveKandevApis(testPage, seedData);

    await testPage.goto("/");
    // Post-overhaul: the Improve Kandev opener moved to the AppSidebar footer.
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });

    // The workflow selector trigger must not render — it would let the user
    // switch away from the kandev contribution workflow that the bootstrap
    // probe already locked in.
    await expect(createDialog.getByTestId("workflow-selector-trigger")).toHaveCount(0);

    // Source-mode switch must be hidden entirely when the repo is locked:
    // neither the Remote nor the None ("scratch") modes can be reached, so
    // the dialog can only ever submit against the bootstrapped kandev repo.
    await expect(createDialog.getByTestId("source-mode-remote")).toHaveCount(0);
    await expect(createDialog.getByTestId("source-mode-scratch")).toHaveCount(0);
    await expect(createDialog.getByTestId("source-mode-workspace")).toHaveCount(0);
  });

  test("submit button stays disabled with bootstrap reason while bootstrap is in flight", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createWorkspace("Improve Kandev");
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    let releaseBootstrap: () => void = () => {};
    const bootstrapHold = new Promise<void>((resolve) => {
      releaseBootstrap = resolve;
    });

    await mockImproveKandevApis(testPage, seedData, { bootstrapHold });

    await testPage.goto("/");
    // Post-overhaul: the Improve Kandev opener moved to the AppSidebar footer.
    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });

    // Banner that explains why the form is partially blocked.
    await expect(
      createDialog.getByText(/Preparing kandev repository in background/i),
    ).toBeVisible();

    // Fill in title and description so every other submit-blocking reason is
    // satisfied — the only remaining blocker should be the pending bootstrap.
    await createDialog.getByTestId("task-title-input").fill("Fix overlapping header");
    await createDialog
      .getByTestId("task-description-input")
      .fill("Steps to reproduce: open kanban on a narrow viewport.");

    const submit = createDialog.getByTestId("submit-start-agent");
    await expect(submit).toBeDisabled();

    // Hover the wrapper so the keyboard-shortcut tooltip (which carries the
    // disabled reason as its description) becomes visible.
    await createDialog.getByTestId("submit-start-agent-wrapper").hover();
    await expect(testPage.getByText(/Preparing kandev repository/i).first()).toBeVisible({
      timeout: 5_000,
    });

    // Pressing the Cmd/Ctrl+Enter shortcut must also be gated by the bootstrap
    // guard — if it bypassed guardedHandleSubmit, the dialog would close and
    // a task would be created with an empty repositoryId.
    await createDialog.getByTestId("task-description-input").focus();
    await testPage.keyboard.press("ControlOrMeta+Enter");
    await expect(createDialog).toBeVisible();
    await expect(
      createDialog.getByText(/Preparing kandev repository in background/i),
    ).toBeVisible();

    // Releasing the bootstrap response transitions the dialog to "ready" and
    // the submit button must become actionable.
    releaseBootstrap();
    await expect(submit).toBeEnabled({ timeout: 10_000 });
  });

  test("offers creating the dedicated workspace when it does not exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Deterministic precondition: the dedicated workspace must not exist.
    const existing = (await apiClient.listWorkspaces()).workspaces.filter(
      (w) => w.name === "Improve Kandev",
    );
    for (const workspace of existing) {
      await apiClient.deleteWorkspace(workspace.id, "Improve Kandev");
    }
    await mockImproveKandevApis(testPage, seedData);
    await testPage.goto("/");

    await testPage.getByTestId("sidebar-improve-kandev-button").click();
    const introDialog = testPage.getByRole("dialog", { name: "Improve Kandev" });
    const choice = introDialog.getByTestId("improve-kandev-create-workspace");
    await expect(choice).toBeVisible({ timeout: 10_000 });
    await expect(choice.getByRole("checkbox")).toBeChecked();

    await testPage.getByTestId("improve-kandev-proceed").click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible({ timeout: 10_000 });
  });

  test("skip-intro users confirm workspace creation before the create form", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const existing = (await apiClient.listWorkspaces()).workspaces.filter(
      (w) => w.name === "Improve Kandev",
    );
    for (const workspace of existing) {
      await apiClient.deleteWorkspace(workspace.id, "Improve Kandev");
    }
    await mockImproveKandevApis(testPage, seedData);
    // The preference must exist before the dialog component mounts (the
    // footer mounts it on page load), so seed it via an init script.
    await testPage.addInitScript(() =>
      window.localStorage.setItem("kandev.improveKandev.skipIntro", "true"),
    );
    await testPage.goto("/");

    await testPage.getByTestId("sidebar-improve-kandev-button").click();

    // The dedicated workspace does not exist: the choice panel gates the
    // create form even for users who dismissed the intro.
    const choicePanel = testPage.getByTestId("improve-kandev-create-workspace-confirm");
    await expect(choicePanel).toBeVisible();
    await expect(testPage.getByTestId("create-task-dialog")).toHaveCount(0);

    await choicePanel.click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible({ timeout: 10_000 });
  });

  test("workflows settings are read-only in the dedicated workspace", async ({
    testPage,
    apiClient,
  }) => {
    const dedicated = await apiClient.createWorkspace("Improve Kandev");

    await testPage.goto(`/settings/workspace/${dedicated.id}/workflows`);

    await expect(testPage.getByText(/cannot be changed/i)).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("add-workflow-button")).toHaveCount(0);
  });

  test("repositories settings are read-only in the dedicated workspace", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Repositories cannot be created in the immutable workspace, so seed under
    // a temporary name and rename (the rename itself is not guarded).
    const staging = await apiClient.createWorkspace("Improve Kandev Setup");
    await apiClient.createRepository(staging.id, seedData.repositoryPath, "main");
    await apiClient.updateWorkspace(staging.id, { name: "Improve Kandev" });
    await apiClient.saveUserSettings({ agent_generated_task_titles: false });
    const dedicated = staging;

    await testPage.goto(`/settings/workspace/${dedicated.id}/repositories`);

    await expect(testPage.getByText(/cannot be changed/i)).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByRole("button", { name: /Add Local Repository/i })).toHaveCount(0);
  });
});
