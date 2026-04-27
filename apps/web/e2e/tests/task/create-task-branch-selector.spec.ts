import path from "node:path";
import fs from "node:fs";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Branch selector behavior with executor types", () => {
  test.describe.configure({ retries: 1 });

  test("branch selector is disabled for local executor with local repo", async ({
    testPage,
    apiClient,
  }) => {
    // Find the system "local" executor and create a profile on it
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local Profile");

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      // Fill title + description
      await testPage.getByTestId("task-title-input").fill("Branch Selector Test");
      await testPage.getByTestId("task-description-input").fill("testing branch selector disable");

      // Select the local executor profile
      const executorSelector = testPage.getByTestId("executor-profile-selector");
      await executorSelector.click();
      await testPage.getByRole("option", { name: /E2E Local Profile/i }).click();

      // Branch selector should be disabled when local executor is selected
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("branch selector re-enables when switching from local to worktree executor", async ({
    testPage,
    apiClient,
  }) => {
    // Find executors
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    const worktreeExec = executors.find((e) => e.type === "worktree");
    if (!localExec || !worktreeExec) {
      test.skip(true, "Need both local and worktree executors");
      return;
    }
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local");
    const worktreeProfile = worktreeExec.profiles?.[0];
    if (!worktreeProfile) {
      test.skip(true, "No worktree profile available");
      return;
    }

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Switch Executor Test");
      await testPage.getByTestId("task-description-input").fill("testing executor switch");

      // Select local executor -> branch should be disabled
      const executorSelector = testPage.getByTestId("executor-profile-selector");
      await executorSelector.click();
      await testPage.getByRole("option", { name: /^E2E Local Local$/i }).click();

      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });

      // Switch to worktree executor -> branch should be enabled
      await executorSelector.click();
      await testPage.getByRole("option", { name: worktreeProfile.name }).click();

      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("branch selector stays enabled for local executor with GitHub URL", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fs = await import("fs");
    const { execSync } = await import("child_process");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };

    // Find local executor and create profile
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Local GitHub URL");

    try {
      // Create a unique repo for this test
      const repoDir = `${backend.tmpDir}/repos/e2e-branch-gh`;
      fs.mkdirSync(repoDir, { recursive: true });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });

      // Seed mock GitHub branches
      await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
        name: "branch-test-owner/branch-test-repo",
        provider: "github",
        provider_owner: "branch-test-owner",
        provider_name: "branch-test-repo",
      });
      await apiClient.mockGitHubAddBranches("branch-test-owner", "branch-test-repo", [
        { name: "main" },
        { name: "develop" },
      ]);

      const kanban = new KanbanPage(testPage);
      await kanban.goto();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      // Toggle to GitHub URL mode
      await testPage.getByTestId("toggle-github-url").click();
      await testPage
        .getByTestId("github-url-input")
        .fill("https://github.com/branch-test-owner/branch-test-repo");

      // Select local executor profile
      const executorSelector = testPage.getByTestId("executor-profile-selector");
      await executorSelector.click();
      await testPage.getByRole("option", { name: /E2E Local GitHub URL/i }).click();

      // Branch selector should NOT be disabled (GitHub URL mode overrides)
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeEnabled({ timeout: 10_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });
});

/**
 * Fresh-branch flow tests.
 *
 * Each test seeds an isolated repo inside backend.tmpDir so the discovery roots
 * check passes, then opens the create-task dialog and exercises the toggle path.
 *
 * Important: the seedData repo is shared across the worker, so we mutate or
 * inspect a NEW repo per test to avoid cross-test interference. We register the
 * repo via apiClient.createRepository so it shows up in the workspace selector.
 */
test.describe("Fresh-branch flow", () => {
  test.describe.configure({ retries: 1 });

  type Setup = {
    repoDir: string;
    repositoryId: string;
    profileId: string;
    profileName: string;
  };

  type ApiClientType = import("../../helpers/api-client").ApiClient;

  async function setupLocalRepo(
    apiClient: ApiClientType,
    backendTmpDir: string,
    workspaceId: string,
    suffix: string,
  ): Promise<(Setup & { repoName: string }) | null> {
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) return null;
    const profileName = `E2E Fresh Branch ${suffix}`;
    const profile = await apiClient.createExecutorProfile(localExec.id, profileName);

    const repoName = `E2E Fresh Repo ${suffix}`;
    const repoDir = path.join(backendTmpDir, "repos", `e2e-fresh-branch-${suffix}`);
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(backendTmpDir);
    execSync("git init -b main", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
    execSync("git checkout -b develop", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "develop"', { cwd: repoDir, env });
    execSync("git checkout main", { cwd: repoDir, env });
    const repo = await apiClient.createRepository(workspaceId, repoDir, "main", { name: repoName });
    return { repoDir, repositoryId: repo.id, profileId: profile.id, profileName, repoName };
  }

  async function openDialogWithLocalProfile(
    testPage: import("@playwright/test").Page,
    profileName: string,
    repoName: string,
  ) {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("Fresh Branch Test");
    await testPage.getByTestId("task-description-input").fill("testing fresh branch");
    await testPage.getByTestId("repository-selector").click();
    await testPage
      .getByRole("option", { name: new RegExp(`^${escapeRe(repoName)}\\b`, "i") })
      .first()
      .click();
    await testPage.getByTestId("executor-profile-selector").click();
    await testPage
      .getByRole("option", { name: new RegExp(`^${escapeRe(profileName)}\\b`, "i") })
      .first()
      .click();
  }

  function escapeRe(s: string) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  test("toggle off (default) — branch selector disabled, placeholder shows current branch", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "default");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      await openDialogWithLocalProfile(testPage, setup.profileName, setup.repoName);
      const toggle = testPage.getByTestId("fresh-branch-toggle");
      await expect(toggle).toBeVisible();
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });
      // Placeholder should show actual current branch (main), not the generic copy.
      await expect(branchSelector).toContainText("main", { timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("toggle on, clean working tree — selector enabled, no discard modal on submit", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "clean");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      await openDialogWithLocalProfile(testPage, setup.profileName, setup.repoName);
      await testPage.getByTestId("fresh-branch-toggle").click();
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
      // Pick the develop base branch so the new branch will fork from it.
      await branchSelector.click();
      await testPage
        .getByRole("option", { name: /develop/ })
        .first()
        .click();

      // Submit and assert the discard modal never appears (clean tree).
      // Wait for the create-task request to fire so we know the submit path
      // really executed and didn't short-circuit before the modal would render.
      const createTaskRequest = testPage.waitForRequest(
        (req) => req.url().endsWith("/api/v1/tasks") && req.method() === "POST",
      );
      await testPage.getByTestId("submit-start-agent").click();
      await createTaskRequest;
      await expect(testPage.getByTestId("discard-local-changes-dialog")).toHaveCount(0);
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("toggle on, dirty working tree — confirm modal lists files; cancel keeps form state", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "dirty");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      // Add an untracked file so `git status` reports it as dirty.
      fs.writeFileSync(path.join(setup.repoDir, "WIP.txt"), "draft");

      await openDialogWithLocalProfile(testPage, setup.profileName, setup.repoName);
      await testPage.getByTestId("fresh-branch-toggle").click();
      // Submit triggers the dirty preflight; the modal lists WIP.txt.
      await testPage.getByTestId("submit-start-agent").click();

      const modal = testPage.getByTestId("discard-local-changes-dialog");
      await expect(modal).toBeVisible({ timeout: 5_000 });
      await expect(testPage.getByTestId("discard-local-changes-files")).toContainText("WIP.txt");

      // Cancel returns to the form with the toggle still on.
      await testPage.getByTestId("discard-local-changes-cancel").click();
      await expect(modal).toBeHidden();
      await expect(testPage.getByTestId("fresh-branch-toggle")).toHaveAttribute(
        "aria-pressed",
        "true",
      );

      // Untracked file must still exist (we didn't confirm).
      expect(fs.existsSync(path.join(setup.repoDir, "WIP.txt"))).toBe(true);
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("dirty working tree — confirm overwrite removes the file and proceeds", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "confirm");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      const wipPath = path.join(setup.repoDir, "WIP-confirm.txt");
      fs.writeFileSync(wipPath, "draft");

      await openDialogWithLocalProfile(testPage, setup.profileName, setup.repoName);
      await testPage.getByTestId("fresh-branch-toggle").click();
      await testPage.getByTestId("submit-start-agent").click();

      await expect(testPage.getByTestId("discard-local-changes-dialog")).toBeVisible({
        timeout: 5_000,
      });
      await testPage.getByTestId("discard-local-changes-confirm").click();

      // After confirm the backend runs reset --hard + clean -fd: the untracked
      // file must be gone.
      await expect.poll(() => fs.existsSync(wipPath), { timeout: 10_000 }).toBe(false);
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("truncated dirty list — many dirty files show '+N more'", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "many");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      // Seed 25 untracked files so the modal cap (20) kicks in.
      for (let i = 0; i < 25; i++) {
        fs.writeFileSync(path.join(setup.repoDir, `f${i}.txt`), "x");
      }
      await openDialogWithLocalProfile(testPage, setup.profileName, setup.repoName);
      await testPage.getByTestId("fresh-branch-toggle").click();
      await testPage.getByTestId("submit-start-agent").click();

      await expect(testPage.getByTestId("discard-local-changes-dialog")).toBeVisible({
        timeout: 5_000,
      });
      await expect(testPage.getByTestId("discard-local-changes-overflow")).toContainText("+5 more");
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("GitHub URL flow hides fresh-branch toggle", async ({ testPage, apiClient }) => {
    const { executors } = await apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) {
      test.skip(true, "No local executor available");
      return;
    }
    const profile = await apiClient.createExecutorProfile(localExec.id, "E2E Fresh Branch GH");
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
      await testPage.getByTestId("task-title-input").fill("Hide toggle");
      await testPage.getByTestId("task-description-input").fill("github url");
      await testPage.getByTestId("toggle-github-url").click();
      await testPage
        .getByTestId("github-url-input")
        .fill("https://github.com/branch-test-owner/branch-test-repo");
      await testPage.getByTestId("executor-profile-selector").click();
      await testPage.getByRole("option", { name: /E2E Fresh Branch GH/i }).click();

      // Toggle is gated behind isLocalExecutor && !useGitHubUrl, so it must not render.
      await expect(testPage.getByTestId("fresh-branch-toggle")).toHaveCount(0);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("non-local executor (worktree) hides fresh-branch toggle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    if (!seedData.worktreeExecutorProfileId) {
      test.skip(true, "No worktree executor profile in seed data");
      return;
    }
    // Look up the actual profile name so the option matcher doesn't depend on
    // the seed naming containing "worktree".
    const { executors } = await apiClient.listExecutors();
    const worktreeProfileName = executors
      .flatMap((e) => e.profiles ?? [])
      .find((p) => p.id === seedData.worktreeExecutorProfileId)?.name;
    if (!worktreeProfileName) {
      test.skip(true, "Could not resolve worktree profile name");
      return;
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
    await testPage.getByTestId("task-title-input").fill("No toggle for worktree");
    await testPage.getByTestId("task-description-input").fill("worktree mode");
    const executorSelector = testPage.getByTestId("executor-profile-selector");
    await executorSelector.click();
    await testPage.getByRole("option", { name: worktreeProfileName }).click();
    await expect(testPage.getByTestId("fresh-branch-toggle")).toHaveCount(0);
  });

  test("switching worktree → local resets the disabled selector to current branch", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const setup = await setupLocalRepo(apiClient, backend.tmpDir, seedData.workspaceId, "switch");
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
      await testPage.getByTestId("task-title-input").fill("Switcheroo");
      await testPage.getByTestId("task-description-input").fill("repro the leftover-branch bug");
      await testPage.getByTestId("repository-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${setup.repoName}\\b`, "i") })
        .first()
        .click();

      // Pick the worktree executor first and select the develop branch.
      const executorSelector = testPage.getByTestId("executor-profile-selector");
      await executorSelector.click();
      await testPage
        .getByRole("option")
        .filter({ hasText: /worktree/i })
        .first()
        .click();
      const branchSelector = testPage.getByTestId("branch-selector");
      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
      await branchSelector.click();
      await testPage
        .getByRole("option", { name: /develop/ })
        .first()
        .click();
      await expect(branchSelector).toContainText("develop");

      // Switch back to local — disabled selector must show the actual current branch (main),
      // not the develop selection that came from the worktree executor.
      await executorSelector.click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${setup.profileName}\\b`, "i") })
        .first()
        .click();
      await expect(branchSelector).toBeDisabled({ timeout: 5_000 });
      await expect(branchSelector).toContainText("main", { timeout: 5_000 });
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });
});
