import { type Page } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { ApiClient } from "../helpers/api-client";
import { PrAssetCapture } from "../helpers/pr-asset-capture";
import { makeGitEnv } from "../helpers/git-helper";
import type { WorkflowStep } from "../../lib/types/http";

export type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
  agentProfileId: string;
  /** Executor profile ID for the worktree executor — use to create tasks with git worktree isolation. */
  worktreeExecutorProfileId: string;
};

export const test = backendFixture.extend<
  {
    testPage: Page;
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
        const probe = await client.rawRequest("GET", "/api/v1/_test/health");
        lastStatus = probe.status;
        if (probe.ok) break;
        if (probe.status !== 404 && probe.status !== 503) {
          lastText = await probe.text();
          break;
        }
        await new Promise((r) => setTimeout(r, 250));
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
      const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
      fs.mkdirSync(repoDir, { recursive: true });
      const gitEnv = makeGitEnv(backend.tmpDir);
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
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
        agentProfileId = agents[0]?.profiles[0]?.id;
        if (agentProfileId) break;
        await new Promise((r) => setTimeout(r, 250));
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
    // Clean up tasks, test-created workflows, and extra agent profiles from
    // previous tests in this worker. Keep the seeded workflow and the seed
    // agent profile so the worker-scoped seedData fixture remains valid.
    await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    await apiClient.cleanupTestProfiles([seedData.agentProfileId]);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
      // Reset to default kanban view. Pipeline-view tests switch this to
      // "graph2", which persists per-workspace; without this reset the next
      // test renders cards with data-testid="pipeline-task-<id>" instead of
      // "task-card-<id>", breaking taskCardByTitle locators.
      kanban_view_mode: "",
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
    await setupPage(page, backend, seedData);
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
    async ({ apiClient }, use) => {
      await apiClient.rawRequest("DELETE", `/api/v1/jira/config`).catch(() => undefined);
      await apiClient.rawRequest("DELETE", `/api/v1/linear/config`).catch(() => undefined);
      await Promise.all([
        apiClient.mockJiraReset().catch(() => undefined),
        apiClient.mockLinearReset().catch(() => undefined),
      ]);
      await use();
    },
    { auto: true },
  ],
});

// Reset the active workspace pointer before every test so that specs which
// do not use the testPage fixture (e.g. API-only routing tests) start from
// a known workspace_id instead of whatever a previous test's completeOnboarding
// call wrote into user_settings. This is idempotent — the testPage fixture
// also calls saveUserSettings, so tests that do use testPage are unaffected.
test.beforeEach(async ({ apiClient, seedData }) => {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
    keyboard_shortcuts: {},
    enable_preview_on_click: false,
    sidebar_views: [],
    kanban_view_mode: "",
  });
});

export { expect } from "@playwright/test";

async function setupPage(page: Page, backend: BackendContext, seedData: SeedData): Promise<void> {
  await page.addInitScript(
    ({
      backendPort,
      repositoryId,
      agentProfileId,
    }: {
      backendPort: string;
      repositoryId: string;
      agentProfileId: string;
    }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      // Pre-seed dialog selections so auto-select effects resolve on their
      // first render cycle instead of waiting for async API chains.
      localStorage.setItem("kandev.dialog.lastRepositoryId", JSON.stringify(repositoryId));
      localStorage.setItem("kandev.dialog.lastAgentProfileId", JSON.stringify(agentProfileId));
      localStorage.setItem("kandev.dialog.lastBranch", JSON.stringify("main"));
      // Set the window global that getBackendConfig() reads for API/WS connections
      // (e2e tests run frontend and backend on separate ports, like dev mode)
      window.__KANDEV_API_PORT = backendPort;

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
      repositoryId: seedData.repositoryId,
      agentProfileId: seedData.agentProfileId,
    },
  );
}
