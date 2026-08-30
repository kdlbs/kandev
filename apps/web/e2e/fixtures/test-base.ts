import { type Page } from "@playwright/test";
import { execFileSync, execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { ApiClient } from "../helpers/api-client";
import { dwell } from "../helpers/causal-waits";
import { PrAssetCapture } from "../helpers/pr-asset-capture";
import { makeGitEnv } from "../helpers/git-helper";
import type { WorkflowStep } from "../../lib/types/http";

const DEFAULT_SIDEBAR_VIEW = {
  id: "view-all-tasks",
  name: "All tasks",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "repository",
  collapsed_groups: [],
};

const AGENT_PROFILE_READY_TIMEOUT_MS = 30_000;
const AGENT_PROFILE_READY_POLL_MS = 250;

function isFetchTransportError(error: unknown): boolean {
  return error instanceof TypeError && /fetch failed|network error/i.test(error.message);
}

async function runWithBackendRecovery<T>(
  backend: BackendContext,
  operation: () => Promise<T>,
): Promise<T> {
  try {
    return await operation();
  } catch (error) {
    if (!isFetchTransportError(error)) throw error;
    // A backend restart can leave the test runner holding a keep-alive socket
    // that closes during setup. All setup operations below are idempotent, so
    // retry the complete reset after replacing that listener once.
    await backend.restart();
    return operation();
  }
}

async function maybeRecoverSeedBackend(
  backend: BackendContext,
  consecutiveFailures: number,
  recoveryAttempted: boolean,
  observation: string,
): Promise<{
  consecutiveFailures: number;
  recoveryAttempted: boolean;
  observation: string;
}> {
  if (consecutiveFailures < 3 || recoveryAttempted) {
    return { consecutiveFailures, recoveryAttempted, observation };
  }

  try {
    await backend.restart();
    return { consecutiveFailures: 0, recoveryAttempted: true, observation };
  } catch (error) {
    return {
      consecutiveFailures,
      recoveryAttempted: true,
      observation: `${observation}; backend recovery failed: ${
        error instanceof Error ? error.message : String(error)
      }`,
    };
  }
}

export type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
  /** Local repository path used by the standard E2E fixture. */
  repositoryPath: string;
  /** Offline bare origin for tests that must exercise remote-ref failures. */
  repositoryRemoteURL: string;
  agentProfileId: string;
  /** Executor profile ID for the worktree executor — use to create tasks with git worktree isolation. */
  worktreeExecutorProfileId: string;
};

async function waitForSeedAgentProfile(
  apiClient: ApiClient,
  backend: BackendContext,
  profileId: string,
): Promise<string> {
  const deadline = Date.now() + AGENT_PROFILE_READY_TIMEOUT_MS;
  let lastObservation = "no response";
  let consecutiveFailures = 0;
  let recoveryAttempted = false;

  while (Date.now() < deadline) {
    try {
      await backend.ensureReady();
      const { agents } = await apiClient.listAgents();
      consecutiveFailures = 0;
      const profiles = agents.flatMap((agent) => agent.profiles ?? []);
      const profile = profiles.find((candidate) => candidate.id === profileId);

      if (profile && profile.enabled !== false) return profile.id;

      if (profile) {
        await apiClient.updateAgentProfile(profileId, { enabled: true });
        return profile.id;
      }

      // Profile-mode tests intentionally restart the same database with the
      // production registry. Its orphan reconciler soft-deletes the E2E mock
      // profile because mock-agent is disabled there. When the E2E profile is
      // restored, create a fresh disposable profile instead of waiting for a
      // user-deleted row that the backend correctly will not resurrect.
      // Virtual families such as Dynamic are settings containers, not
      // launchable profile owners. They may be the first API row after the
      // registry keeps them visible with routing disabled, so never use one
      // as the replacement-profile target. Use the stable ID because the
      // display name is localized and capitalized.
      const agent =
        agents.find((candidate) => candidate.name === "mock-agent") ??
        agents.find((candidate) => candidate.id !== "dynamic");
      if (agent) {
        const replacement = await apiClient.createAgentProfile(agent.id, "mock-fast", {
          model: "mock-fast",
        });
        return replacement.id;
      }

      lastObservation =
        `seed profile ${profileId} was not returned by /api/v1/agents ` +
        `(available profiles: ${profiles.map((candidate) => candidate.id).join(", ") || "none"})`;
    } catch (error) {
      lastObservation = error instanceof Error ? error.message : String(error);
      consecutiveFailures += 1;
      // A backend restart can leave an old keep-alive socket in the test
      // runner. After several consecutive transport failures, restart the
      // isolated worker backend once so the next poll uses a fresh listener.
      const recovery = await maybeRecoverSeedBackend(
        backend,
        consecutiveFailures,
        recoveryAttempted,
        lastObservation,
      );
      consecutiveFailures = recovery.consecutiveFailures;
      recoveryAttempted = recovery.recoveryAttempted;
      lastObservation = recovery.observation;
    }

    await dwell(
      AGENT_PROFILE_READY_POLL_MS,
      "poll-interval",
      "agent profile readiness has no event notification",
    );
  }

  throw new Error(
    `E2E backend did not expose enabled seed agent profile ${profileId} within ` +
      `${AGENT_PROFILE_READY_TIMEOUT_MS}ms (${lastObservation})`,
  );
}

export const test = backendFixture.extend<
  {
    testPage: Page;
    tabletTestPage: Page;
    prCapture: PrAssetCapture;
    /**
     * Auto fixture that resets integration mock state and any persisted
     * Jira/Linear configs at the top of every test. Auto fixtures run
     * automatically — unlike a top-level `test.beforeEach` registered in this
     * module, which Playwright only fires for tests defined in the same file.
     */
    integrationCleanup: void;
  },
  { apiClient: ApiClient; seedData: SeedData }
>({
  // Worker-scoped API client
  apiClient: [
    async ({ backend }, use) => {
      const client = new ApiClient(backend.baseUrl);
      // Confirm the E2E mock routes mounted. They are gated by KANDEV_E2E_MOCK
      // in fixtures/backend.ts; if the env var isn't propagating, /api/v1/_test
      // returns 404 and every session-driven test would fail with a confusing
      // network error.
      //
      // The backend's `/health` endpoint can flip green before every router
      // group has been registered — the office testharness mount runs from
      // a post-init goroutine on some boot paths. Poll the test-harness
      // health for a short window before raising the "not mounted" error so
      // a startup race doesn't poison the worker-scoped fixture for every
      // subsequent test in the file.
      const probeDeadline = Date.now() + 10_000;
      let lastStatus = 0;
      let lastText = "";
      while (Date.now() < probeDeadline) {
        try {
          const probe = await client.rawRequest("GET", "/api/v1/_test/health");
          lastStatus = probe.status;
          if (probe.ok) break;
          if (probe.status !== 404 && probe.status !== 503) {
            lastText = await probe.text();
            break;
          }
        } catch (error) {
          lastText = error instanceof Error ? error.message : String(error);
        }
        await dwell(
          250,
          "poll-interval",
          "sampling interval for the mock-harness health probe above; the backend is still booting and there is no page, let alone a socket, to receive a ready signal on",
        );
      }
      if (lastStatus === 404) {
        throw new Error(
          "E2E mock harness not mounted: /api/v1/_test/health returned 404 after 10s of polling. " +
            "Verify KANDEV_E2E_MOCK=true is propagated to the backend (fixtures/backend.ts) " +
            "and that the backend was rebuilt after the testharness package was added.",
        );
      }
      if (lastStatus !== 200) {
        throw new Error(`E2E mock harness probe failed: ${lastStatus} ${lastText}`);
      }
      await use(client);
    },
    { scope: "worker" },
  ],

  // Worker-scoped seed data: creates workspace, workflow (from template), discovers steps,
  // and sets up a local git repository for agent execution workspace.
  // The repo is created inside backend.tmpDir (the backend's HOME) so that
  // discoveryRoots() allows branch listing (isPathAllowed check).
  seedData: [
    async ({ apiClient, backend }, use) => {
      const workspace = await apiClient.createWorkspace("E2E Workspace");
      const workflow = await apiClient.createWorkflow(workspace.id, "E2E Workflow", "simple");

      const { steps } = await apiClient.listWorkflowSteps(workflow.id);
      const sorted = steps.sort((a, b) => a.position - b.position);
      const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

      // Create a minimal git repository inside backend.tmpDir (the backend's HOME).
      // This ensures discoveryRoots() allows the path for branch listing.
      // It also has an offline bare origin with main, so tests can distinguish
      // a genuinely missing remote ref from an unavailable Git host.
      const remoteDir = path.join(backend.tmpDir, "repos", "e2e-remote.git");
      const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
      fs.mkdirSync(path.dirname(remoteDir), { recursive: true });
      fs.mkdirSync(repoDir, { recursive: true });
      const gitEnv = makeGitEnv(backend.tmpDir);
      execSync(`git init --bare -b main "${remoteDir}"`, { env: gitEnv });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      fs.writeFileSync(
        path.join(repoDir, "walkthrough_base.txt"),
        "line 1: WALKTHROUGH_UNCHANGED\nline 2: seeded on main\n",
      );
      execSync("git add walkthrough_base.txt", { cwd: repoDir, env: gitEnv });
      execSync('git commit -m "init"', { cwd: repoDir, env: gitEnv });
      execSync(`git remote add origin "file://${remoteDir}"`, { cwd: repoDir, env: gitEnv });
      execSync("git push origin main", { cwd: repoDir, env: gitEnv });
      const repo = await apiClient.createRepository(workspace.id, repoDir);

      // Agent registry seeding (runInitialAgentSetup → discovery) is
      // synchronous before `/health` flips green in main.go, BUT
      // `EnsureInitialAgentProfiles` failures are non-fatal (warn-only),
      // so a discovery hiccup leaves the registry permanently empty
      // until the next restart. Poll long enough to ride out a slow
      // discovery walk and capture diagnostics if it really fails so
      // the next debug run isn't blind.
      let agentProfileId: string | undefined;
      let lastAgentCount = -1;
      const agentsDeadline = Date.now() + 30_000;
      while (Date.now() < agentsDeadline) {
        const { agents } = await apiClient.listAgents();
        lastAgentCount = agents.length;
        // Virtual families (for example Dynamic) sort before concrete
        // providers but do not own executable profiles. Search all returned
        // families instead of assuming the first row is launchable.
        agentProfileId = agents
          .filter((agent) => agent.id !== "dynamic")
          .flatMap((agent) => agent.profiles ?? [])[0]?.id;
        if (agentProfileId) break;
        await dwell(
          250,
          "poll-interval",
          "sampling interval for the seeded-agent-profile poll above; seeding is asynchronous backend work read back over HTTP, with no page in this fixture",
        );
      }
      if (!agentProfileId) {
        throw new Error(
          `E2E seed failed: no agent profile available after 30s of polling ` +
            `(listAgents returned ${lastAgentCount} agent(s) on the last attempt). ` +
            `Likely cause: runInitialAgentSetup in main.go warn-logged a discovery ` +
            `failure and the backend started anyway with an empty registry. ` +
            `Check the backend log for "Failed to run initial agent setup".`,
        );
      }

      // Find the worktree executor's profile so tests can opt in to worktree-based sessions.
      const { executors } = await apiClient.listExecutors();
      const worktreeExec = executors.find((e) => e.type === "worktree");
      const worktreeExecutorProfileId = worktreeExec?.profiles?.[0]?.id;
      if (!worktreeExecutorProfileId) {
        throw new Error("E2E seed failed: no worktree executor profile available");
      }

      await use({
        workspaceId: workspace.id,
        workflowId: workflow.id,
        startStepId: startStep.id,
        steps: sorted,
        repositoryId: repo.id,
        repositoryPath: repoDir,
        repositoryRemoteURL: `file://${remoteDir}`,
        agentProfileId,
        worktreeExecutorProfileId,
      });
    },
    { scope: "worker" },
  ],

  // Per-test page with baseURL pointing to worker's frontend.
  // Resets user settings to the E2E workspace/workflow before each test so that
  // SSR always resolves to the correct workspace regardless of what commitSettings
  // may have written during previous tests.
  testPage: async ({ browser, backend, apiClient, seedData }, use) => {
    await backend.ensureReady();
    // A suite-level test may restart the worker backend after the worker-scoped
    // seed fixture ran. Health only proves that the listener is serving; it does
    // not prove that the persisted seed profile is present and enabled for the
    // task-create dialog. Re-establish that invariant before opening a page.
    seedData.agentProfileId = await waitForSeedAgentProfile(
      apiClient,
      backend,
      seedData.agentProfileId,
    );
    await runWithBackendRecovery(backend, async () => {
      // Clean up tasks, test-created workflows, and extra agent profiles from
      // previous tests in this worker. Keep the seeded workflow and the seed
      // agent profile so the worker-scoped seedData fixture remains valid.
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
      await apiClient.updateWorkspace(seedData.workspaceId, { default_agent_profile_id: "" });
      await apiClient.cleanupTestProfiles([seedData.agentProfileId]);

      await apiClient.saveUserSettings({
        workspace_id: seedData.workspaceId,
        workflow_filter_id: seedData.workflowId,
        keyboard_shortcuts: {},
        enable_preview_on_click: false,
        confirm_task_archive: true,
        agent_generated_task_titles: false,
        mcp_task_agent_profile_default: "current_task",
        sidebar_views: [DEFAULT_SIDEBAR_VIEW],
        sidebar_active_view_id: DEFAULT_SIDEBAR_VIEW.id,
        sidebar_draft: null,
        saved_layouts: [],
        lsp_auto_start_languages: [],
        lsp_auto_install_languages: [],
        lsp_server_configs: {},
        task_create_last_used: {
          repository_id: seedData.repositoryId,
          branch: "main",
          agent_profile_id: seedData.agentProfileId,
          workflow_ids_by_workspace: { [seedData.workspaceId]: seedData.workflowId },
        },
        // Reset to default kanban view. Pipeline-view tests switch this to
        // "graph2", which persists per-workspace; without this reset the next
        // test renders cards with data-testid="pipeline-task-<id>" instead of
        // "task-card-<id>", breaking taskCardByTitle locators.
        kanban_view_mode: "",
        // Keep startup routing deterministic for tests that open bare home.
        startup_page: "task_overview",
        // Reset to the default (off). Prevent-auto-start tests flip this via
        // saveUserSettings; without this reset it would leak into unrelated
        // tests running later in the same worker.
        prevent_auto_start_agent_on_open: false,
        // Reset to the default (off). Anchored-bar tests flip this via
        // saveUserSettings; without this reset it would leak into unrelated
        // tests running later in the same worker.
        show_anchored_prompt_bar: false,
        show_scroll_to_last_prompt: true,
        show_scroll_to_start: false,
        show_transcript_auto_scroll_control: true,
        tasks_list_sort: "updated_desc",
        tasks_list_group: "state",
      });
    });
    const context = await browser.newContext({
      baseURL: backend.frontendUrl,
    });
    const page = await context.newPage();
    if (process.env.E2E_BROWSER_CONSOLE === "1") {
      page.on("console", (msg) => {
        console.log(`[browser:${msg.type()}]`, msg.text());
      });
    }
    await setupPage(page, backend);
    await use(page);
    await context.close();
  },

  tabletTestPage: async ({ browser, backend, testPage }, use) => {
    // Depend on testPage so its per-test backend and settings reset runs before
    // this specialized context is created.
    void testPage;
    const context = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 900, height: 900 },
      hasTouch: true,
      isMobile: false,
    });
    const page = await context.newPage();
    await setupPage(page, backend);
    await use(page);
    await context.close();
  },

  // PR asset capture — gated behind CAPTURE_PR_ASSETS env var.
  // When enabled, provides screenshot/recording helpers for PR descriptions.
  // Destructure in tests that need it: { testPage, prCapture }
  prCapture: async ({ testPage }, use, testInfo) => {
    const capture = new PrAssetCapture(testPage, testInfo.file);
    await use(capture);
    capture.flush();
  },

  integrationCleanup: [
    async ({ apiClient, backend, seedData }, use) => {
      await backend.ensureReady();
      const scoped = `workspace_id=${encodeURIComponent(seedData.workspaceId)}`;
      await apiClient.rawRequest("DELETE", `/api/v1/jira/config?${scoped}`).catch(() => undefined);
      await apiClient
        .rawRequest("DELETE", `/api/v1/linear/config?${scoped}`)
        .catch(() => undefined);
      await apiClient
        .rawRequest("DELETE", `/api/v1/gitlab/config?${scoped}`)
        .catch(() => undefined);
      await apiClient.deleteAllSentryInstances(seedData.workspaceId).catch(() => undefined);
      await apiClient
        .rawRequest("DELETE", `/api/v1/azure-devops/config?${scoped}`)
        .catch(() => undefined);
      await apiClient.rawRequest("DELETE", `/api/v1/jira/config`).catch(() => undefined);
      await apiClient.rawRequest("DELETE", `/api/v1/linear/config`).catch(() => undefined);
      await Promise.all([
        // Provider-focused specs reuse and mutate the worker-scoped seed row.
        // Restore its local-only identity before the next test; otherwise a
        // removed mock remote plus stale provider metadata breaks workspace prep.
        apiClient
          .updateRepository(seedData.repositoryId, {
            provider: "",
            provider_host: "",
            provider_owner: "",
            provider_name: "",
          })
          .catch(() => undefined),
        apiClient.mockJiraReset().catch(() => undefined),
        apiClient.mockLinearReset().catch(() => undefined),
        apiClient.mockSentryReset().catch(() => undefined),
        apiClient.mockAzureDevOpsReset().catch(() => undefined),
        apiClient.mockGitLabReset(seedData.workspaceId).catch(() => undefined),
        apiClient.clearGitLabRepositoryRemote(seedData.repositoryId).catch(() => undefined),
      ]);
      // GitLab reset removes origin from the shared seed checkout. Restore the
      // fixture's offline origin before the test so pull-enabled worktree
      // preparation starts from the same valid repository state every time.
      restoreSeedRepositoryOrigin(seedData);
      try {
        await use();
      } finally {
        await apiClient.clearGitLabRepositoryRemote(seedData.repositoryId).catch(() => undefined);
        restoreSeedRepositoryOrigin(seedData);
      }
    },
    { auto: true },
  ],
});

/**
 * Restores the fixture's offline origin after a GitLab E2E cleanup removes it.
 * Refresh the remote-tracking refs because branch recovery must offer remote
 * branches, not only the local branch that remains checked out.
 */
export function restoreSeedRepositoryOrigin(seedData: SeedData) {
  const baseArgs = ["-C", seedData.repositoryPath, "remote"];
  try {
    execFileSync("git", [...baseArgs, "set-url", "origin", seedData.repositoryRemoteURL], {
      stdio: "ignore",
    });
  } catch {
    execFileSync("git", [...baseArgs, "add", "origin", seedData.repositoryRemoteURL], {
      stdio: "ignore",
    });
  }
  execFileSync("git", ["-C", seedData.repositoryPath, "fetch", "--no-tags", "origin"], {
    stdio: "ignore",
  });
}

/** Restores the shared seed checkout to its clean main branch. */
export function resetSeedRepositoryCheckout(seedData: SeedData, tmpDir: string) {
  const env = makeGitEnv(tmpDir);
  execFileSync("git", ["-C", seedData.repositoryPath, "checkout", "-f", "main"], {
    env,
    stdio: "ignore",
  });
  execFileSync("git", ["-C", seedData.repositoryPath, "reset", "--hard", "main"], {
    env,
    stdio: "ignore",
  });
  execFileSync("git", ["-C", seedData.repositoryPath, "clean", "-fd"], {
    env,
    stdio: "ignore",
  });
}

/** Points the seed repository at an empty remote whose HEAD cannot resolve. */
export function pointSeedRepositoryAtUnresolvedOrigin(seedData: SeedData, tmpDir: string) {
  const remoteDir = path.join(
    tmpDir,
    "repos",
    `e2e-unresolved-remote-${Date.now()}-${process.pid}.git`,
  );
  fs.mkdirSync(path.dirname(remoteDir), { recursive: true });
  execFileSync("git", ["init", "--bare", "--initial-branch=main", remoteDir], {
    env: makeGitEnv(tmpDir),
    stdio: "ignore",
  });
  try {
    execFileSync(
      "git",
      ["-C", seedData.repositoryPath, "remote", "set-url", "origin", `file://${remoteDir}`],
      { env: makeGitEnv(tmpDir), stdio: "ignore" },
    );
  } catch {
    execFileSync(
      "git",
      ["-C", seedData.repositoryPath, "remote", "add", "origin", `file://${remoteDir}`],
      { env: makeGitEnv(tmpDir), stdio: "ignore" },
    );
  }
}

/** Points the seed repository at a valid cached checkout with an unreachable origin. */
export function pointSeedRepositoryAtFailingOrigin(seedData: SeedData, tmpDir: string) {
  const remoteDir = path.join(
    tmpDir,
    "repos",
    `e2e-failing-remote-${Date.now()}-${process.pid}.git`,
  );
  try {
    execFileSync(
      "git",
      ["-C", seedData.repositoryPath, "remote", "set-url", "origin", `file://${remoteDir}`],
      { env: makeGitEnv(tmpDir), stdio: "ignore" },
    );
  } catch {
    execFileSync(
      "git",
      ["-C", seedData.repositoryPath, "remote", "add", "origin", `file://${remoteDir}`],
      { env: makeGitEnv(tmpDir), stdio: "ignore" },
    );
  }
}

// Reset the active workspace pointer before every test so that specs which
// do not use the testPage fixture (e.g. API-only routing tests) start from
// a known workspace_id instead of whatever a previous test's completeOnboarding
// call wrote into user_settings. This is idempotent — the testPage fixture
// also calls saveUserSettings, so tests that do use testPage are unaffected.
test.beforeEach(async ({ apiClient, backend, seedData }) => {
  await runWithBackendRecovery(backend, async () => {
    await apiClient.updateWorkspace(seedData.workspaceId, { default_agent_profile_id: "" });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      confirm_task_archive: true,
      agent_generated_task_titles: false,
      mcp_task_agent_profile_default: "current_task",
      sidebar_views: [DEFAULT_SIDEBAR_VIEW],
      sidebar_active_view_id: DEFAULT_SIDEBAR_VIEW.id,
      sidebar_draft: null,
      saved_layouts: [],
      // Status-surface specs opt in from their local beforeEach hooks; unrelated
      // tests start from the portable setting's default-off state.
      app_status_bar_enabled: false,
      lsp_auto_start_languages: [],
      lsp_auto_install_languages: [],
      lsp_server_configs: {},
      kanban_view_mode: "",
      tasks_list_show_details: false,
      tasks_list_sort: "updated_desc",
      tasks_list_group: "state",
      startup_page: "task_overview",
      show_anchored_prompt_bar: false,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: false,
      show_transcript_auto_scroll_control: true,
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
        workflow_ids_by_workspace: { [seedData.workspaceId]: seedData.workflowId },
      },
    });
  });
});

export { expect } from "@playwright/test";

async function setupPage(page: Page, backend: BackendContext): Promise<void> {
  await page.addInitScript(
    ({ backendPort }: { backendPort: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      // Set the window global that getBackendConfig() reads for API/WS connections
      // (e2e tests run frontend and backend on separate ports, like dev mode)
      window.__KANDEV_API_PORT = backendPort;
      window.__KANDEV_E2E_EXPOSE_STORE__ = true;

      // Replace native Notification with a capture stub so e2e runs never
      // pop OS-level toasts on the developer's machine. Tests that want to
      // assert read window.__kandevTestNotifications via the helpers in
      // e2e/helpers/notifications-capture.ts. permission stays "granted"
      // so the WS handler at apps/web/lib/ws/handlers/notifications.ts
      // (which early-returns when not granted) still runs its full logic.
      const captured: { title: string; body?: string }[] = [];
      (
        window as unknown as { __kandevTestNotifications: typeof captured }
      ).__kandevTestNotifications = captured;
      class NotificationStub {
        static permission: NotificationPermission = "granted";
        static async requestPermission(): Promise<NotificationPermission> {
          return "granted";
        }
        title: string;
        body?: string;
        constructor(title: string, opts?: NotificationOptions) {
          this.title = title;
          this.body = opts?.body;
          captured.push({ title, body: opts?.body });
        }
        close(): void {}
        addEventListener(): void {}
        removeEventListener(): void {}
        dispatchEvent(): boolean {
          return false;
        }
      }
      Object.defineProperty(window, "Notification", {
        configurable: true,
        writable: true,
        value: NotificationStub,
      });
    },
    {
      backendPort: String(backend.port),
    },
  );
}
