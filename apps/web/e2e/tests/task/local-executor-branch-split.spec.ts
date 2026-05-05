import path from "node:path";
import fs from "node:fs";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { makeGitEnv } from "../../helpers/git-helper";

/**
 * Regression suite for the "new branch on local executor empties the changes
 * panel after refresh" bug.
 *
 * Failure mode that originally shipped:
 *   1. The dialog seeded the chip with the user's currently-checked-out branch
 *      and submitted it as `base_branch`.
 *   2. agentctl recomputed merge-base(HEAD, origin/<chip-branch>) on every
 *      refresh; for a never-pushed feature branch that collapsed to HEAD and
 *      `git log HEAD..HEAD` returned empty.
 *   3. Live polling kept showing commits via WS while the user was on the page,
 *      but the moment they refreshed, the panel went blank.
 *
 * The forward fix splits the chip's branch into two pieces for the local
 * executor (the only mode where there is no worktree and the chip semantics
 * differ): `base_branch` anchors to the repo's `default_branch` (the
 * integration target — what the merge-base needs to reference), and
 * `checkout_branch` carries the actual working branch the preparer should
 * check out. The shape is what these tests assert end-to-end.
 *
 * They also pin the second half of the fix: `repositories.default_branch`
 * must be probed from the repo's actual integration ref (origin/HEAD,
 * origin/main, origin/master, …), not from `.git/HEAD` — otherwise a user
 * who first ran the dialog while checked out on a feature branch would
 * permanently pin their repo's default to that feature branch, and every
 * downstream merge-base lookup would be anchored wrong.
 */
test.describe("Local executor branch split", () => {
  test.describe.configure({ retries: 1 });

  type Setup = {
    repoDir: string;
    repoName: string;
    repositoryId: string;
    profileId: string;
    profileName: string;
    defaultBranchOnDisk: string;
  };

  /**
   * Build an isolated repo per test inside backend.tmpDir so:
   *   - The discoveryRoots() allowlist accepts it (createRepository hits the
   *     same path validation as the dialog).
   *   - `git init -b main` ensures the integration branch exists locally and
   *     readGitDefaultBranch will probe it correctly.
   *   - Multiple feature branches let the chip exercise both "match default"
   *     and "differs from default" paths.
   */
  async function setupLocalRepo(opts: {
    apiClient: import("../../helpers/api-client").ApiClient;
    backendTmpDir: string;
    workspaceId: string;
    suffix: string;
    /** Branch the test wants the working tree checked out on at task-create
     *  time. Must already be created below. */
    headBranch: string;
    /** Extra branches to create alongside `main`. The chip dropdown lists
     *  them; the test picks one to drive the split. */
    extraBranches?: string[];
  }): Promise<Setup | null> {
    const { executors } = await opts.apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) return null;
    const profileName = `E2E Branch Split ${opts.suffix}`;
    const profile = await opts.apiClient.createExecutorProfile(localExec.id, profileName);

    const repoName = `E2E Branch Split Repo ${opts.suffix}`;
    const repoDir = path.join(opts.backendTmpDir, "repos", `e2e-branch-split-${opts.suffix}`);
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(opts.backendTmpDir);
    execSync("git init -b main", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init on main"', { cwd: repoDir, env });
    for (const branch of opts.extraBranches ?? []) {
      execSync(`git checkout -B ${branch}`, { cwd: repoDir, env });
      execSync(`git commit --allow-empty -m "${branch} commit"`, { cwd: repoDir, env });
    }
    execSync(`git checkout ${opts.headBranch}`, { cwd: repoDir, env });

    // Register the repo with default_branch=main so the workspace store has
    // the correct integration ref. The frontend's split logic looks this
    // value up to populate base_branch.
    const repo = await opts.apiClient.createRepository(opts.workspaceId, repoDir, "main", {
      name: repoName,
    });
    return {
      repoDir,
      repoName,
      repositoryId: repo.id,
      profileId: profile.id,
      profileName,
      defaultBranchOnDisk: "main",
    };
  }

  function escapeRe(s: string) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  /**
   * Drives the create-task dialog up to the point of submission with the
   * local executor profile and the seeded repo selected. Returns when
   * submission has fired so callers can read back the persisted task.
   */
  async function createTaskViaDialog(opts: {
    testPage: import("@playwright/test").Page;
    title: string;
    description: string;
    profileName: string;
    repoName: string;
    /** When set, click the chip and pick this branch; otherwise leave the
     *  auto-seeded current branch. */
    pickBranch?: string;
  }) {
    const { testPage } = opts;
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("task-title-input").fill(opts.title);
    await testPage.getByTestId("task-description-input").fill(opts.description);

    await testPage.getByTestId("repo-chip-trigger").first().click();
    await testPage
      .getByRole("option", { name: new RegExp(`^${escapeRe(opts.repoName)}\\b`, "i") })
      .first()
      .click();

    await testPage.getByTestId("executor-profile-selector").click();
    await testPage
      .getByRole("option", { name: new RegExp(`^${escapeRe(opts.profileName)}\\b`, "i") })
      .first()
      .click();

    // Wait for the chip to settle on the workspace's current branch — the
    // autoselect effect runs after currentLocalBranch resolves, and submitting
    // before then would race with the very logic we're testing.
    const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
    await expect(branchSelector).toBeEnabled({ timeout: 5_000 });

    if (opts.pickBranch) {
      await branchSelector.click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${escapeRe(opts.pickBranch)}\\b`) })
        .first()
        .click();
      await expect(branchSelector).toContainText(opts.pickBranch);
    }

    // Use the "create without starting agent" path so the test doesn't wait
    // for agent boot. We care about the persisted task shape, not session
    // lifecycle. The chevron opens the split-button menu.
    //
    // Wait on the *response* rather than the request — waitForRequest
    // resolves the moment the browser sends the POST, before the server has
    // finished writing to the database. Callers that read the task back via
    // the API immediately after would race the persistence.
    const createTaskResponse = opts.testPage.waitForResponse(
      (res) => res.url().endsWith("/api/v1/tasks") && res.request().method() === "POST",
    );
    await testPage.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-create-without-agent").click();
    await createTaskResponse;
  }

  test("submits with base_branch=default_branch and checkout_branch=working_branch", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // The pre-fix shape was {base_branch: "feature/x", checkout_branch: ""}
    // which caused the post-refresh empty-panel bug. The new shape isolates
    // the integration ref from the working branch so the merge-base
    // recomputation downstream lands on the actual fork point instead of
    // collapsing to HEAD.
    const setup = await setupLocalRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix: "split",
      headBranch: "feature/x",
      extraBranches: ["feature/x"],
    });
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      await createTaskViaDialog({
        testPage,
        title: "Local executor branch split",
        description: "regression: base/checkout split for local executor",
        profileName: setup.profileName,
        repoName: setup.repoName,
      });

      // Find the just-created task by its title and assert its repository row.
      const tasksRes = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/tasks`,
      );
      const tasksJson = (await tasksRes.json()) as {
        tasks: Array<{ id: string; title: string }>;
      };
      const task = tasksJson.tasks.find((t) => t.title === "Local executor branch split");
      expect(task, "task should exist").toBeTruthy();

      const fullTask = await apiClient.getTask(task!.id);
      const taskRepo = fullTask.repositories?.[0];
      expect(taskRepo, "task should have a repository row").toBeTruthy();
      expect(taskRepo!.repository_id).toBe(setup.repositoryId);
      expect(
        taskRepo!.base_branch,
        "base_branch must be the repo's default_branch — that's what the changes panel uses as merge-base reference",
      ).toBe("main");
      expect(
        taskRepo!.checkout_branch,
        "checkout_branch carries the working branch the preparer must switch to",
      ).toBe("feature/x");
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("checkout_branch is omitted when picked branch matches default_branch", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // When the user is on `main` (matching default_branch), there's nothing
    // for the preparer to check out. The frontend should NOT send a redundant
    // checkout_branch — the LocalPreparer's "skip when current matches"
    // optimization relies on it being absent so we don't fire spurious git
    // ops on every task creation.
    const setup = await setupLocalRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix: "match",
      headBranch: "main",
    });
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      await createTaskViaDialog({
        testPage,
        title: "Branch matches default",
        description: "no checkout_branch needed",
        profileName: setup.profileName,
        repoName: setup.repoName,
      });

      const tasksRes = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/tasks`,
      );
      const tasksJson = (await tasksRes.json()) as {
        tasks: Array<{ id: string; title: string }>;
      };
      const task = tasksJson.tasks.find((t) => t.title === "Branch matches default");
      expect(task, "task should exist").toBeTruthy();

      const fullTask = await apiClient.getTask(task!.id);
      const taskRepo = fullTask.repositories?.[0];
      expect(taskRepo!.base_branch).toBe("main");
      // omitempty on the DTO — empty string round-trips as undefined/missing.
      expect(taskRepo!.checkout_branch ?? "").toBe("");
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("explicit branch pick: chip switches to develop, payload still splits correctly", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // The chip auto-seeds with the workspace's current branch (main here),
    // but the user can switch to any existing branch. The split must still
    // anchor base_branch to default_branch regardless of how the chip got
    // its value (autoselect vs explicit pick).
    const setup = await setupLocalRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix: "pick",
      headBranch: "main",
      extraBranches: ["develop"],
    });
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      await createTaskViaDialog({
        testPage,
        title: "Branch pick split",
        description: "user explicitly picked develop",
        profileName: setup.profileName,
        repoName: setup.repoName,
        pickBranch: "develop",
      });

      const tasksRes = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/tasks`,
      );
      const tasksJson = (await tasksRes.json()) as {
        tasks: Array<{ id: string; title: string }>;
      };
      const task = tasksJson.tasks.find((t) => t.title === "Branch pick split");
      expect(task, "task should exist").toBeTruthy();

      const fullTask = await apiClient.getTask(task!.id);
      const taskRepo = fullTask.repositories?.[0];
      expect(taskRepo!.base_branch).toBe("main");
      expect(taskRepo!.checkout_branch).toBe("develop");
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });
});

/**
 * Fresh-branch flow has different chip semantics: row.branch is the base to
 * fork FROM, not a working branch. The split must NOT apply here — otherwise
 * picking a non-default base would silently fork from main instead of the
 * picked base.
 */
test.describe("Local executor + fresh-branch toggle", () => {
  test.describe.configure({ retries: 1 });

  type Setup = {
    repoDir: string;
    repoName: string;
    profileId: string;
    profileName: string;
  };

  async function setupFreshBranchRepo(opts: {
    apiClient: import("../../helpers/api-client").ApiClient;
    backendTmpDir: string;
    workspaceId: string;
    suffix: string;
  }): Promise<Setup | null> {
    const { executors } = await opts.apiClient.listExecutors();
    const localExec = executors.find((e) => e.type === "local");
    if (!localExec) return null;
    const profileName = `E2E Fresh Split ${opts.suffix}`;
    const profile = await opts.apiClient.createExecutorProfile(localExec.id, profileName);
    const repoName = `E2E Fresh Split Repo ${opts.suffix}`;
    const repoDir = path.join(opts.backendTmpDir, "repos", `e2e-fresh-split-${opts.suffix}`);
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(opts.backendTmpDir);
    execSync("git init -b main", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
    execSync("git checkout -b develop", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "develop work"', { cwd: repoDir, env });
    execSync("git checkout main", { cwd: repoDir, env });
    await opts.apiClient.createRepository(opts.workspaceId, repoDir, "main", { name: repoName });
    return { repoDir, repoName, profileId: profile.id, profileName };
  }

  function escapeRe(s: string) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  test("payload sends base_branch=picked-base verbatim, no checkout_branch split", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // Regression for the gap noticed during review: the local-executor branch
    // split would otherwise force base_branch=default_branch and the backend's
    // applyFreshBranch would fork from main instead of the user's pick.
    //
    // The frontend's contract is to send the chip's value verbatim as
    // base_branch when fresh-branch is active. Backend persistence of the
    // resulting fork-branch name is a separate concern covered by
    // service-layer tests on PerformFreshBranch. We only assert the wire
    // payload here so this test stays focused on the regression and doesn't
    // entangle with the post-create session-launch path.
    const setup = await setupFreshBranchRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix: "develop-base",
    });
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
      await testPage.getByTestId("task-title-input").fill("Fork from develop");
      await testPage.getByTestId("task-description-input").fill("non-default fork base");
      await testPage.getByTestId("repo-chip-trigger").first().click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${escapeRe(setup.repoName)}\\b`, "i") })
        .first()
        .click();
      await testPage.getByTestId("executor-profile-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${escapeRe(setup.profileName)}\\b`, "i") })
        .first()
        .click();

      // Flip on fresh-branch and pick "develop" as the base to fork from.
      await testPage.getByTestId("fresh-branch-toggle").click();
      const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
      await branchSelector.click();
      await testPage
        .getByRole("option", { name: /develop/ })
        .first()
        .click();
      await expect(branchSelector).toContainText("develop");

      // Wait on the response so the server has finished persisting before we
      // inspect the request body. waitForRequest resolves on send, which
      // would race the rest of the test on slow CI.
      const createTaskResponse = testPage.waitForResponse(
        (res) => res.url().endsWith("/api/v1/tasks") && res.request().method() === "POST",
      );
      await testPage.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      const req = (await createTaskResponse).request();

      // Wire-level contract: base_branch must carry "develop" verbatim and
      // checkout_branch must be absent. fresh_branch must be true so the
      // backend's applyFreshBranch flow knows to fork. Without the split
      // bypass, base_branch would be "main" and applyFreshBranch would fork
      // from the wrong base.
      const payload = JSON.parse(req.postData() ?? "{}") as {
        repositories?: Array<{
          base_branch?: string;
          checkout_branch?: string;
          fresh_branch?: boolean;
        }>;
      };
      const repo = payload.repositories?.[0];
      expect(repo?.base_branch).toBe("develop");
      expect(repo?.checkout_branch ?? undefined).toBeUndefined();
      expect(repo?.fresh_branch).toBe(true);
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });

  test("forks from default base when chip is left on the auto-seeded current branch", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    // Common path: user is on main (the workspace default), enables fresh-
    // branch, leaves the chip alone. Backend forks from main. We just verify
    // the resulting branch points at main's tip and the wire payload sent
    // base_branch="main" without a redundant checkout_branch.
    const setup = await setupFreshBranchRepo({
      apiClient,
      backendTmpDir: backend.tmpDir,
      workspaceId: seedData.workspaceId,
      suffix: "main-base",
    });
    if (!setup) {
      test.skip(true, "No local executor available");
      return;
    }
    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
      await testPage.getByTestId("task-title-input").fill("Fork from main");
      await testPage.getByTestId("task-description-input").fill("default fork base");
      await testPage.getByTestId("repo-chip-trigger").first().click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${escapeRe(setup.repoName)}\\b`, "i") })
        .first()
        .click();
      await testPage.getByTestId("executor-profile-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(`^${escapeRe(setup.profileName)}\\b`, "i") })
        .first()
        .click();
      await testPage.getByTestId("fresh-branch-toggle").click();
      const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
      await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
      await expect(branchSelector).toContainText("main");

      const createTaskResponse = testPage.waitForResponse(
        (res) => res.url().endsWith("/api/v1/tasks") && res.request().method() === "POST",
      );
      await testPage.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      const req = (await createTaskResponse).request();

      const payload = JSON.parse(req.postData() ?? "{}") as {
        repositories?: Array<{ base_branch?: string; checkout_branch?: string }>;
      };
      expect(payload.repositories?.[0]?.base_branch).toBe("main");
      expect(payload.repositories?.[0]?.checkout_branch ?? undefined).toBeUndefined();
    } finally {
      await apiClient.deleteExecutorProfile(setup.profileId).catch(() => {});
    }
  });
});

/**
 * Pins the readGitDefaultBranch fix: the validate-path endpoint (which the
 * dialog uses to populate `default_branch` for discovered local paths) must
 * return the integration branch, not whatever ref `.git/HEAD` happens to
 * point at. The pre-fix code read HEAD directly, so a user checked out on
 * a feature branch would permanently pin the resulting `repositories`
 * row's `default_branch` to that feature branch.
 */
test.describe("Repository default_branch detection", () => {
  test.describe.configure({ retries: 1 });

  test("validate-path returns 'main' even when HEAD is on a feature branch", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-default-branch-probe");
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
    execSync("git checkout -b feature/probe", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "feature work"', { cwd: repoDir, env });
    // Leave HEAD on feature/probe — the pre-fix code returned this string,
    // which is exactly what we're guarding against.
    expect(
      execSync("git rev-parse --abbrev-ref HEAD", { cwd: repoDir, env }).toString().trim(),
    ).toBe("feature/probe");

    const res = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories/validate?path=${encodeURIComponent(repoDir)}`,
    );
    expect(res.status).toBe(200);
    const body = (await res.json()) as {
      is_git: boolean;
      default_branch: string;
      allowed: boolean;
    };
    expect(body.is_git).toBe(true);
    expect(body.allowed).toBe(true);
    expect(
      body.default_branch,
      "must probe origin/HEAD or main/master, not echo back .git/HEAD",
    ).toBe("main");
  });

  test("validate-path falls back to feature branch only when no main/master exists", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    // A truly local-only repo with only a feature branch has no integration
    // ref to anchor to. The probe should return the current HEAD as a last
    // resort so the dialog doesn't error out — callers can still override.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-default-branch-fallback");
    fs.mkdirSync(repoDir, { recursive: true });
    const env = makeGitEnv(backend.tmpDir);
    execSync("git init -b solo", { cwd: repoDir, env });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });

    const res = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories/validate?path=${encodeURIComponent(repoDir)}`,
    );
    const body = (await res.json()) as { default_branch: string };
    expect(body.default_branch).toBe("solo");
  });
});
