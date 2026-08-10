import { expect, type Page, test as base } from "@playwright/test";
import { execFileSync } from "node:child_process";
import {
  execInContainer,
  startSSHServer,
  stopSSHServer,
  type SSHServerHandle,
} from "../helpers/ssh";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { hasSSHContainerSupport, buildE2ESSHImage, SSH_E2E_IMAGE_TAG } from "./ssh-image";
import { ApiClient } from "../helpers/api-client";
import { makeGitEnv } from "../helpers/git-helper";
import { startHTTPGitFixture } from "../helpers/http-git-server";
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
  /** Removes the worker-scoped HTTP Git fixture and its SSH URL rewrite. */
  sshGitFixtureCleanup: () => Promise<void>;
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
  { testPage: Page; _sshRuntimeReset: void },
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
        test.skip(true, "Docker daemon not reachable; skipping SSH E2E worker");
        return;
      }
      buildE2ESSHImage();

      // Per-worker sshd container.
      const sshWorkDir = path.join(backend.tmpDir, "ssh");
      const sshTarget = startSSHServer(workerInfo.workerIndex, SSH_E2E_IMAGE_TAG, sshWorkDir);

      // ssh-keyscan reports the ed25519 fingerprint, but the Go ssh client may
      // negotiate a different key type — meaning the pinned fingerprint must
      // be the one *kandev* observes, not the one we scanned. Hit the
      // permissive test endpoint once to capture the canonical value.
      try {
        const observed = await apiClient.testSSHConnection({
          name: "ssh-fixture-observe",
          host: sshTarget.host,
          port: sshTarget.port,
          user: sshTarget.user,
          identity_source: "file",
          identity_file: sshTarget.identityFile,
        });
        if (!observed.success || !observed.fingerprint) {
          throw new Error(
            `SSH fixture: probe failed (${observed.error ?? "no error"}); ` +
              `steps=${JSON.stringify(observed.steps)}`,
          );
        }
        sshTarget.hostFingerprint = observed.fingerprint;
      } catch (e) {
        stopSSHServer(sshTarget);
        throw e;
      }

      let seed: Omit<SSHSeedData, "sshTarget"> | undefined;
      try {
        seed = await seedSSHWorkspace(apiClient, backend, sshTarget);
        await use({ ...seed, sshTarget });
      } finally {
        await seed?.sshGitFixtureCleanup().catch(() => undefined);
        stopSSHServer(sshTarget);
      }
    },
    { scope: "worker", timeout: 180_000 },
  ],

  testPage: async ({ browser, backend, apiClient, seedData }, use) => {
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

  _sshRuntimeReset: [
    async ({ apiClient, seedData }, use) => {
      await resetSSHRuntime(apiClient, seedData);
      try {
        await use();
      } finally {
        await resetSSHRuntime(apiClient, seedData);
      }
    },
    { auto: true },
  ],
});

async function resetSSHRuntime(apiClient: ApiClient, seedData: SSHSeedData) {
  await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
  let emptySince = 0;
  await expect
    .poll(
      async () => {
        const count = (await apiClient.listSSHSessions(seedData.sshExecutorId)).length;
        if (count > 0) {
          emptySince = 0;
          return false;
        }
        if (emptySince === 0) emptySince = Date.now();
        return Date.now() - emptySince >= 1_500;
      },
      {
        message: "previous SSH runtime rows should stay empty before the next test",
        timeout: 60_000,
      },
    )
    .toBe(true);
}

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

  // SSH executor clones inside the remote container, so the fixture must be
  // reachable from that container rather than using a host-only file:// URL.
  // The HTTP server is disposable and the SSH target rewrites the canonical
  // provider URL to its Docker-bridge endpoint without changing Git origin.
  const gitFixture = await startHTTPGitFixture(backend.tmpDir, "e2e-ssh");
  const localRepoDir = path.join(backend.tmpDir, "repos", "e2e-ssh-repo");
  const localGitEnv = makeGitEnv(backend.tmpDir);
  execFileSync(
    "git",
    ["clone", path.join(backend.tmpDir, "fixture", "e2e-ssh.git"), localRepoDir],
    { env: localGitEnv },
  );
  const rewriteKey = gitFixture.gitConfigEnvVars.find(
    ({ key }) => key === "GIT_CONFIG_KEY_0",
  )?.value;
  const rewriteValue = gitFixture.gitConfigEnvVars.find(
    ({ key }) => key === "GIT_CONFIG_VALUE_0",
  )?.value;
  if (!rewriteKey || !rewriteValue) {
    await gitFixture.close();
    throw new Error("SSH E2E seed: HTTP Git fixture did not provide its URL rewrite");
  }
  const sshGitFixtureCleanup = async () => {
    try {
      execInContainer(sshTarget, ["git", "config", "--system", "--unset-all", rewriteKey]);
    } catch {
      // The target may already have exited after a failed fixture setup.
    }
    await gitFixture.close();
  };
  try {
    execInContainer(sshTarget, ["git", "config", "--system", rewriteKey, rewriteValue]);
    const repo = await apiClient.createRepository(workspace.id, gitFixture.checkoutPath, "main", {
      name: "fixture/e2e-ssh",
      provider: "gitlab",
      provider_host: "https://gitlab.com",
      provider_owner: "fixture",
      provider_name: "e2e-ssh",
    });

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
      sshGitFixtureCleanup,
    };
  } catch (error) {
    await sshGitFixtureCleanup().catch(() => undefined);
    throw error;
  }
}

export { expect } from "@playwright/test";
export const test = sshTest;
export { base };
