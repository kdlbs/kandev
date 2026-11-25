import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/ssh-test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import type {
  SSHIdentity,
  SSHRoute,
  SSHObservation,
  SSHEnrollment,
} from "../../../lib/types/http-ssh-connections";

test("managed key enrollment survives restart and launches a reusable SSH executor", async ({
  backend,
  seedData,
  apiClient,
  testPage,
}) => {
  test.setTimeout(300_000);
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  const request = async <T>(path: string, body?: unknown): Promise<T> => {
    const response = await fetch(
      `${backend.baseUrl}/api/v1/ssh/managed${path}`,
      body === undefined
        ? {}
        : {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
          },
    );
    expect(response.ok, await response.clone().text()).toBe(true);
    return response.json() as Promise<T>;
  };
  try {
    const key = await request<SSHIdentity>("/identities", {
      request_id: randomUUID(),
      label: "managed runtime key",
      kind: "generate",
    });
    const bootstrap = await request<SSHIdentity>("/identities", {
      request_id: randomUUID(),
      label: "existing bootstrap key",
      kind: "file",
      reference: seedData.sshTarget.identityFile,
    });
    let route = await request<SSHRoute>("/connections", {
      request_id: randomUUID(),
      expected_version: 0,
      connection: {
        name: "managed target",
        host: seedData.sshTarget.host,
        port: seedData.sshTarget.port,
        user: seedData.sshTarget.user,
        identity_id: key.id,
      },
    });
    const root = `/connections/${route.destination.id}`;
    const observation = await request<SSHObservation>(`${root}/observe`, {
      expected_stamp: route.stamp,
    });
    route = await request<SSHRoute>(`${root}/trust`, {
      expected_stamp: observation.stamp,
      receipt: observation.receipt,
      fingerprint: observation.fingerprint,
    });
    const enrollment = await request<SSHEnrollment>(`${root}/enroll`, {
      request_id: randomUUID(),
      expected_stamp: route.stamp,
      key_fingerprint: key.fingerprint,
      mode: "identity",
      bootstrap_identity_id: bootstrap.id,
    });
    expect(enrollment.state).toBe("verified");
    await backend.restart();
    route = await request<SSHRoute>(root);
    expect(route.latest_enrollment?.state).toBe("verified");
    route = await request<SSHRoute>(`${root}/test`, { expected_stamp: route.stamp });
    const executor = await apiClient.createSSHExecutor("Managed SSH", {
      ssh_connection_id: route.destination.id,
      ssh_connection_stamp: route.stamp,
    });
    const profile = await apiClient.createExecutorProfile(executor.id, {
      name: "Managed first",
      config: {},
      env_vars: [],
      prepare_script: "",
      cleanup_script: "",
    });
    await apiClient.createExecutorProfile(executor.id, {
      name: "Managed second",
      config: {},
      env_vars: [],
      prepare_script: "",
      cleanup_script: "",
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "managed SSH launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: profile.id,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Managed SSH launch");
    await testPage.goto(`/settings/executors/ssh-connections?tab=identities&id=${key.id}`);
    await expect(
      testPage.getByRole("button", { name: "Delete identity", exact: true }),
    ).toBeDisabled();
    await expect(
      testPage.getByRole("link", { name: `Session: ${task.session_id}`, exact: true }),
    ).toHaveAttribute("href", `/t/${task.id}?sessionId=${task.session_id}`);
    await testPage.getByRole("link", { name: `Session: ${task.session_id}`, exact: true }).click();
    await expect(testPage).toHaveURL(`/t/${task.id}?sessionId=${task.session_id}`);

    const before = (await apiClient.listSSHSessions(executor.id)).find(
      (row) => row.task_id === task.id,
    );
    expect(before?.remote_agentctl_port).toBeGreaterThan(0);
    const changed = await fetch(`${backend.baseUrl}/api/v1/ssh/managed${root}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        expected_version: route.destination.version,
        connection: { ...route.destination, host: "other.invalid" },
      }),
    });
    expect(changed.status).toBe(409);
    await backend.restart();
    await expect
      .poll(
        async () =>
          (await apiClient.listSSHSessions(executor.id)).find((row) => row.task_id === task.id)
            ?.remote_agentctl_port,
        { timeout: 60_000 },
      )
      .toBe(before!.remote_agentctl_port);
    await expect
      .poll(
        async () => {
          const row = (await apiClient.listSSHSessions(executor.id)).find(
            (candidate) => candidate.task_id === task.id,
          );
          if (!row?.local_forward_port) return false;
          try {
            const response = await fetch(`http://127.0.0.1:${row.local_forward_port}/health`, {
              signal: AbortSignal.timeout(2_000),
            });
            return response.ok;
          } catch {
            return false;
          }
        },
        {
          timeout: 60_000,
          message: "retained agentctl is reachable through the restored SSH tunnel",
        },
      )
      .toBe(true);
  } finally {
    await testPage.close();
    try {
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    } finally {
      await release();
    }
  }
});
