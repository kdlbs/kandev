import { type Page, test as base } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { buildE2EImage, E2E_IMAGE_TAG, hasDocker, removeKandevContainers } from "./docker-probe";
import { ApiClient } from "../helpers/api-client";
import { makeGitEnv } from "../helpers/git-helper";
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
};

/**
 * Docker E2E test base. Skips the entire worker when no Docker daemon is
 * available so contributors without Docker can still run the rest of the
 * suite. When Docker is available, builds the kandev-agent:e2e image once
 * and pre-seeds a local_docker executor profile pointing at it.
 */
export const dockerTest = backendFixture.extend<
  { testPage: Page },
  { apiClient: ApiClient; seedData: DockerSeedData }
>({
  apiClient: [
    async ({ backend }, use) => {
      const client = new ApiClient(backend.baseUrl);
      await use(client);
    },
    { scope: "worker" },
  ],

  seedData: [
    async ({ apiClient, backend }, use, workerInfo) => {
      if (!hasDocker()) {
        workerInfo.skip(true, "Docker daemon not reachable; skipping Docker E2E worker");
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

      // The Docker executor clones inside the container, so the repository
      // needs a fetchable remote URL. Set up a local bare repo as the remote
      // and push the working repo's main branch into it. Using `file://` keeps
      // the test offline.
      const remoteDir = path.join(backend.tmpDir, "repos", "e2e-docker-remote.git");
      fs.mkdirSync(path.dirname(remoteDir), { recursive: true });
      const gitEnv = makeGitEnv(backend.tmpDir);
      execSync(`git init --bare -b main "${remoteDir}"`, { env: gitEnv });

      const repoDir = path.join(backend.tmpDir, "repos", "e2e-docker-repo");
      fs.mkdirSync(repoDir, { recursive: true });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
      execSync(`git remote add origin "file://${remoteDir}"`, { cwd: repoDir, env: gitEnv });
      execSync("git push origin main", { cwd: repoDir, env: gitEnv });
      const repo = await apiClient.createRepository(workspace.id, repoDir);

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
        env_vars: [],
      });

      try {
        await use({
          workspaceId: workspace.id,
          workflowId: workflow.id,
          startStepId: startStep.id,
          steps: sorted,
          repositoryId: repo.id,
          agentProfileId,
          dockerExecutorProfileId: dockerProfile.id,
        });
      } finally {
        removeKandevContainers();
      }
    },
    { scope: "worker", timeout: 120_000 },
  ],

  testPage: async ({ browser, backend, apiClient, seedData }, use) => {
    await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();
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
        localStorage.setItem("kandev.dialog.lastRepositoryId", JSON.stringify(repositoryId));
        localStorage.setItem("kandev.dialog.lastAgentProfileId", JSON.stringify(agentProfileId));
        localStorage.setItem("kandev.dialog.lastBranch", JSON.stringify("main"));
        window.__KANDEV_API_PORT = backendPort;
      },
      {
        backendPort: String((backend as BackendContext).port),
        repositoryId: seedData.repositoryId,
        agentProfileId: seedData.agentProfileId,
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
