import { type Page, test as base } from "@playwright/test";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import {
  buildE2EImage,
  E2E_IMAGE_TAG,
  hasDocker,
  removeScopedKandevContainers,
  waitForScopedKandevContainersRemoved,
} from "./docker-probe";
import { ApiClient } from "../helpers/api-client";
import { makeGitEnv } from "../helpers/git-helper";
import { startHTTPGitFixture } from "../helpers/http-git-server";
import type { WorkflowStep } from "../../lib/types/http";

export type DockerSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
  agentProfileId: string;
  /** Executor profile of type local_docker, pre-built for the e2e image. */
  dockerExecutorProfileId: string;
  /** Git transport rewrite needed by custom Docker profiles in this fixture. */
  gitConfigEnvVars: Array<{ key: string; value: string }>;
};

/**
 * Docker E2E test base. Skips the entire worker when no Docker daemon is
 * available so contributors without Docker can still run the rest of the
 * suite. When Docker is available, builds the kandev-agent:e2e image once
 * and pre-seeds a local_docker executor profile pointing at it.
 */
export const dockerTest = backendFixture.extend<
  { testPage: Page; dockerCleanup: void },
  { apiClient: ApiClient; seedData: DockerSeedData }
>({
  apiClient: [
    async ({ backend }, use) => {
      const client = new ApiClient(backend.baseUrl);
      await use(client);
    },
    { scope: "worker" },
  ],

  // Keep reset and Docker cleanup in an automatic test-scoped fixture. A
  // fixture teardown is ordered around every test, including API-only tests
  // and serial suites, so a stopped container cannot leak into the next test.
  dockerCleanup: [
    async ({ apiClient, seedData }, use) => {
      const reset = async () => {
        try {
          await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
        } finally {
          await removeScopedKandevContainers();
        }
      };

      await reset();
      try {
        await use();
      } finally {
        await reset();
      }
    },
    { auto: true },
  ],

  seedData: [
    async ({ apiClient, backend }, use, _workerInfo) => {
      if (!hasDocker()) {
        test.skip(true, "Docker daemon not reachable; skipping Docker E2E worker");
        return;
      }
      buildE2EImage();

      const workspace = await apiClient.createWorkspace("E2E Docker Workspace");
      const workflow = await apiClient.createWorkflow(
        workspace.id,
        "E2E Docker Workflow",
        "simple",
      );

      const { steps } = await apiClient.listWorkflowSteps(workflow.id);
      const sorted = steps.sort((a, b) => a.position - b.position);
      const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

      // Docker clones from inside a sibling container, so a backend-local
      // file:// source is neither a valid clone endpoint nor a daemon-visible
      // bind source. Use the bridge-reachable HTTP fixture and preserve the
      // canonical GitLab identity through the executor-local URL rewrite.
      const gitFixture = await startHTTPGitFixture(backend.tmpDir, "e2e-docker");
      const repoDir = path.join(backend.tmpDir, "repos", "e2e-docker-repo");
      execFileSync(
        "git",
        ["clone", path.join(backend.tmpDir, "fixture", "e2e-docker.git"), repoDir],
        { env: makeGitEnv(backend.tmpDir) },
      );

      try {
        // Register the canonical-URL checkout so the Docker executor resolves
        // the HTTP fixture through its profile rewrite. The separate local
        // clone above remains the mutable push target used by LSP tests.
        const repo = await apiClient.createRepository(
          workspace.id,
          gitFixture.checkoutPath,
          "main",
          {
            name: "fixture/e2e-docker",
            provider: "gitlab",
            provider_host: "https://gitlab.com",
            provider_owner: "fixture",
            provider_name: "e2e-docker",
          },
        );

        const { agents } = await apiClient.listAgents();
        const mock = agents.find((a) => a.name === "mock-agent");
        const agentProfileId = mock?.profiles[0]?.id;
        if (!agentProfileId) {
          throw new Error("Docker E2E seed failed: mock-agent profile missing");
        }

        const { executors } = await apiClient.listExecutors();
        const dockerExec = executors.find((e) => e.type === "local_docker");
        if (!dockerExec) {
          throw new Error("Docker E2E seed failed: local_docker executor not registered");
        }
        const dockerProfile = await apiClient.createExecutorProfile(dockerExec.id, {
          name: "E2E Docker",
          config: { image_tag: E2E_IMAGE_TAG },
          prepare_script: "",
          cleanup_script: "",
          env_vars: gitFixture.gitConfigEnvVars,
        });

        await use({
          workspaceId: workspace.id,
          workflowId: workflow.id,
          startStepId: startStep.id,
          steps: sorted,
          repositoryId: repo.id,
          agentProfileId,
          dockerExecutorProfileId: dockerProfile.id,
          gitConfigEnvVars: gitFixture.gitConfigEnvVars,
        });
      } finally {
        await gitFixture.close();
        // The backend owns containers created by the test task. Do not sweep
        // the daemon here: another E2E shard may be using it concurrently.
        await waitForScopedKandevContainersRemoved();
      }
    },
    { scope: "worker", timeout: 120_000 },
  ],

  testPage: async ({ browser, backend, apiClient, seedData }, use) => {
    await backend.ensureReady();
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      lsp_auto_start_languages: [],
      lsp_auto_install_languages: [],
      lsp_server_configs: {},
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
      },
    });
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();
    await page.addInitScript(
      ({ backendPort }: { backendPort: string }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = backendPort;
      },
      {
        backendPort: String((backend as BackendContext).port),
      },
    );
    await use(page);
    await context.close();
  },
});

export { expect } from "@playwright/test";
export const test = dockerTest;
// Re-export base for convenience to keep import paths consistent across specs.
export { base };
