import { type Page, test as base } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { hasSSHContainerSupport, buildE2ESSHImage, SSH_E2E_IMAGE_TAG } from "./ssh-image";
import { startSSHServer, stopSSHServer, type SSHServerHandle } from "../helpers/ssh";
import { ApiClient } from "../helpers/api-client";
import { makeGitEnv } from "../helpers/git-helper";
import type { WorkflowStep } from "../../lib/types/http";

export type SSHSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
  agentProfileId: string;
  /**
   * A fully wired SSH executor pointing at the worker's sshd container with
   * the host fingerprint already trusted. Tests that exercise the
   * test-then-trust gate should create their own executor instead of using
   * this one.
   */
  sshExecutorId: string;
  sshExecutorProfileId: string;
  /** Live handle to the sshd container the executor connects to. */
  sshTarget: SSHServerHandle;
};

/**
 * SSH E2E test base. Spins up a real sshd container per worker, generates
 * keys, observes the host fingerprint, and pre-seeds an SSH executor with
 * that fingerprint already trusted — most specs use this to skip the UI
 * test-then-trust flow and get straight to the assertion they care about.
 *
 * Skips the entire worker when no Docker daemon is reachable so contributors
 * without Docker can still run the chromium project.
 */
export const sshTest = backendFixture.extend<
  { testPage: Page },
  { apiClient: ApiClient; seedData: SSHSeedData }
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
      if (!hasSSHContainerSupport()) {
        workerInfo.skip(true, "Docker daemon not reachable; skipping SSH E2E worker");
        return;
      }
      buildE2ESSHImage();

      // Per-worker sshd container.
      const sshWorkDir = path.join(backend.tmpDir, "ssh");
      const sshTarget = startSSHServer(workerInfo.workerIndex, SSH_E2E_IMAGE_TAG, sshWorkDir);

      try {
        const seed = await seedSSHWorkspace(apiClient, backend, sshTarget);
        await use({ ...seed, sshTarget });
      } finally {
        stopSSHServer(sshTarget);
      }
    },
    { scope: "worker", timeout: 180_000 },
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

async function seedSSHWorkspace(
  apiClient: ApiClient,
  backend: BackendContext,
  sshTarget: SSHServerHandle,
): Promise<Omit<SSHSeedData, "sshTarget">> {
  const workspace = await apiClient.createWorkspace("E2E SSH Workspace");
  const workflow = await apiClient.createWorkflow(workspace.id, "E2E SSH Workflow", "simple");

  const { steps } = await apiClient.listWorkflowSteps(workflow.id);
  const sorted = steps.sort((a, b) => a.position - b.position);
  const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

  // SSH executor clones inside the remote container; needs a fetchable URL.
  // Same offline-bare-repo trick the Docker fixture uses.
  const remoteDir = path.join(backend.tmpDir, "repos", "e2e-ssh-remote.git");
  fs.mkdirSync(path.dirname(remoteDir), { recursive: true });
  const gitEnv = makeGitEnv(backend.tmpDir);
  execSync(`git init --bare -b main "${remoteDir}"`, { env: gitEnv });

  const repoDir = path.join(backend.tmpDir, "repos", "e2e-ssh-repo");
  fs.mkdirSync(repoDir, { recursive: true });
  execSync("git init -b main", { cwd: repoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
  execSync(`git remote add origin "file://${remoteDir}"`, { cwd: repoDir, env: gitEnv });
  execSync("git push origin main", { cwd: repoDir, env: gitEnv });
  const repo = await apiClient.createRepository(workspace.id, repoDir);

  const { agents } = await apiClient.listAgents();
  const mock = agents.find((a) => a.name === "mock-agent");
  const agentProfileId = mock?.profiles[0]?.id;
  if (!agentProfileId) throw new Error("SSH E2E seed: mock-agent profile missing");

  const sshExecutor = await apiClient.createSSHExecutor("E2E SSH Target", {
    ssh_host: sshTarget.host,
    ssh_port: String(sshTarget.port),
    ssh_user: sshTarget.user,
    ssh_identity_source: "file",
    ssh_identity_file: sshTarget.identityFile,
    ssh_host_fingerprint: sshTarget.hostFingerprint,
  });

  const profile = await apiClient.createExecutorProfile(sshExecutor.id, {
    name: "E2E SSH",
    config: {},
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });

  return {
    workspaceId: workspace.id,
    workflowId: workflow.id,
    startStepId: startStep.id,
    steps: sorted,
    repositoryId: repo.id,
    agentProfileId,
    sshExecutorId: sshExecutor.id,
    sshExecutorProfileId: profile.id,
  };
}

export { expect } from "@playwright/test";
export const test = sshTest;
export { base };
