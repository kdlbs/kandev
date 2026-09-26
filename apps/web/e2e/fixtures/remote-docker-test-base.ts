import type { Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import {
  buildE2EImage,
  E2E_IMAGE_TAG,
  hasDocker,
  removeScopedKandevContainers,
} from "./docker-probe";
import { buildE2ESSHImage, SSH_E2E_IMAGE_TAG } from "./ssh-image";
import { ApiClient } from "../helpers/api-client";
import { makeGitEnv } from "../helpers/git-helper";
import { startHTTPGitFixture } from "../helpers/http-git-server";
import { startRemoteDockerHost } from "../helpers/remote-docker";
import { stopSSHServer, type SSHServerHandle } from "../helpers/ssh";
import type { WorkflowStep } from "../../lib/types/http";

export type RemoteDockerSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
  agentProfileId: string;
  /** Executor of type remote_docker, pointed at the SSH host below. */
  remoteDockerExecutorId: string;
  remoteDockerExecutorProfileId: string;
  /** The SSH host whose Docker daemon runs the task containers. */
  remoteHost: SSHServerHandle;
};

/**
 * Remote Docker E2E base.
 *
 * The SSH host shares the machine's Docker socket and network namespace, so a
 * task container created through that socket publishes agentctl on a loopback
 * the executor's SSH forward can reach. See helpers/remote-docker.ts for why
 * both are required.
 */
export const remoteDockerTest = backendFixture.extend<
  { testPage: Page; remoteDockerCleanup: void },
  { apiClient: ApiClient; seedData: RemoteDockerSeedData }
>({
  apiClient: [
    async ({ backend }, use) => {
      await use(new ApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],

  remoteDockerCleanup: [
    async ({ apiClient, seedData }, use) => {
      const reset = async () => {
        try {
          await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
        } finally {
          await removeScopedKandevContainers();
        }
      };
      await reset();
      await use();
      await reset();
    },
    { auto: true },
  ],

  seedData: [
    async ({ backend }, use, workerInfo) => {
      if (!hasDocker()) {
        remoteDockerTest.skip(
          true,
          "Docker daemon not reachable; skipping remote Docker E2E worker",
        );
        return;
      }
      buildE2EImage();
      buildE2ESSHImage();

      const workDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-rdocker-"));
      const remoteHost = startRemoteDockerHost(workerInfo.workerIndex, SSH_E2E_IMAGE_TAG, workDir);
      try {
        const apiClient = new ApiClient(backend.baseUrl);

        // Take the fingerprint from the backend's own dial rather than from
        // ssh-keyscan, matching the SSH fixture. The scanner and the Go client
        // can disagree about which host key they end up pinning, and the
        // launch then fails as a host-key change. This also exercises the
        // remote Docker probe end to end before any task runs.
        const observed = await apiClient.testRemoteDockerConnection({
          name: "E2E Remote Docker Target",
          host: remoteHost.host,
          port: remoteHost.port,
          user: remoteHost.user,
          identity_source: "file",
          identity_file: remoteHost.identityFile,
        });
        if (!observed.success || !observed.fingerprint) {
          throw new Error(
            `Remote Docker fixture: probe failed (${observed.error ?? "no error"}); ` +
              `steps=${JSON.stringify(observed.steps)}`,
          );
        }
        remoteHost.hostFingerprint = observed.fingerprint;

        const seed = await seedRemoteDockerWorkspace(apiClient, backend, remoteHost);
        await use({ ...seed, remoteHost });
      } finally {
        stopSSHServer(remoteHost);
      }
    },
    { scope: "worker" },
  ],
});

async function seedRemoteDockerWorkspace(
  apiClient: ApiClient,
  backend: BackendContext,
  remoteHost: SSHServerHandle,
): Promise<Omit<RemoteDockerSeedData, "remoteHost">> {
  const workspace = await apiClient.createWorkspace("E2E Remote Docker Workspace");
  const workflow = await apiClient.createWorkflow(
    workspace.id,
    "E2E Remote Docker Workflow",
    "simple",
  );
  const { steps } = await apiClient.listWorkflowSteps(workflow.id);
  const sorted = steps.sort((a, b) => a.position - b.position);
  const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

  // The task container clones from inside a sibling container, exactly as the
  // local Docker fixture does, so it uses the same bridge-reachable HTTP
  // fixture and the same profile-local URL rewrite.
  const gitFixture = await startHTTPGitFixture(backend.tmpDir, "e2e-rdocker");
  execFileSync(
    "git",
    [
      "clone",
      path.join(backend.tmpDir, "fixture", "e2e-rdocker.git"),
      path.join(backend.tmpDir, "repos", "e2e-rdocker-repo"),
    ],
    { env: makeGitEnv(backend.tmpDir) },
  );

  try {
    const repo = await apiClient.createRepository(workspace.id, gitFixture.checkoutPath, "main", {
      name: "fixture/e2e-rdocker",
      provider: "gitlab",
      provider_host: "https://gitlab.com",
      provider_owner: "fixture",
      provider_name: "e2e-rdocker",
    });

    const { agents } = await apiClient.listAgents();
    const agentProfileId = agents.find((a) => a.name === "mock-agent")?.profiles[0]?.id;
    if (!agentProfileId) {
      throw new Error("Remote Docker E2E seed failed: mock-agent profile missing");
    }

    const executor = await apiClient.createExecutor("E2E Remote Docker Target", "remote_docker", {
      ssh_host: remoteHost.host,
      ssh_port: String(remoteHost.port),
      ssh_user: remoteHost.user,
      ssh_identity_source: "file",
      ssh_identity_file: remoteHost.identityFile,
      ssh_host_fingerprint: remoteHost.hostFingerprint,
    });

    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "E2E Remote Docker",
      config: { image_tag: E2E_IMAGE_TAG },
      prepare_script: "",
      cleanup_script: "",
      env_vars: gitFixture.gitConfigEnvVars,
    });

    return {
      workspaceId: workspace.id,
      workflowId: workflow.id,
      startStepId: startStep.id,
      steps: sorted,
      repositoryId: repo.id,
      agentProfileId,
      remoteDockerExecutorId: executor.id,
      remoteDockerExecutorProfileId: profile.id,
    };
  } finally {
    await gitFixture.close();
  }
}

export const test = remoteDockerTest;
export { expect } from "@playwright/test";
