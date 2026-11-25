import type { ApiClient } from "../../helpers/api-client";
import { test, expect } from "../../fixtures/ssh-test-base";
import { execInContainer } from "../../helpers/ssh";
import { waitForSessionDone } from "../../helpers/session";

async function executorTestRequest(apiClient: ApiClient, id: string) {
  const config = (await apiClient.getExecutor(id)).config!;
  return {
    ssh_connection_id: config.ssh_connection_id,
    expected_stamp: config.ssh_connection_stamp,
  };
}

test.describe("ssh test-endpoint contract", () => {
  // F2 — happy-path step ordering against a reachable sshd container.
  test("checks SSH, the remote platform, and the agentctl cache in order", async ({
    apiClient,
    seedData,
  }) => {
    const result = await apiClient.testSSHConnection({
      name: "F2",
      ...(await executorTestRequest(apiClient, seedData.sshExecutorId)),
    });

    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe(seedData.sshTarget.hostFingerprint);
    expect(result.os).toBe("Linux");
    expect(result.arch).toBe("x86_64");
    expect(result.platform).toBe("linux/amd64");

    const stepNames = result.steps.map((s) => s.name);
    expect(stepNames).toEqual([
      "SSH handshake",
      "Probe remote",
      "Verify platform",
      "Verify agentctl cache",
    ]);
    for (const step of result.steps) {
      expect(step.success, `step ${step.name} should succeed`).toBe(true);
    }
  });

  test("rejects a non-object body with 400", async ({ apiClient }) => {
    const res = await apiClient.rawRequest("POST", "/api/v1/ssh/test", "not json");
    expect(res.status).toBe(400);
  });

  test("rejects inline and mixed target selectors", async ({ apiClient, seedData }) => {
    const saved = await executorTestRequest(apiClient, seedData.sshExecutorId);
    for (const body of [
      { name: "inline", host: "example.invalid" },
      { ...saved, host: "example.invalid" },
    ]) {
      const response = await apiClient.rawRequest("POST", "/api/v1/ssh/test", body);
      expect(response.status).toBe(400);
    }
  });

  test("rejects a stale revision without reaching the remote platform", async ({
    apiClient,
    seedData,
  }) => {
    const request = await executorTestRequest(apiClient, seedData.sshExecutorId);
    const result = await apiClient.testSSHConnection({
      name: "stale",
      ...request,
      expected_stamp: "stale",
    });
    expect(result.success).toBe(false);
    expect(result.steps ?? []).toEqual([]);
    expect(result.error).toBe("ssh_resource_changed");
  });

  // F5 — agentctl cache step states (cached vs upload-on-launch).
  test("agentctl cache step reports cached after a real launch primes it", async ({
    apiClient,
    seedData,
  }) => {
    // Wipe any prior agentctl + sha256 sidecar from the per-worker container
    // so the "before" probe sees a fresh-install state regardless of which
    // tests ran earlier in this worker.
    execInContainer(seedData.sshTarget, [
      "rm",
      "-f",
      "/home/kandev/.kandev/bin/agentctl",
      "/home/kandev/.kandev/bin/agentctl.sha256",
    ]);

    // First test run: agentctl has never been uploaded, status = "needs_upload"
    // (the test endpoint only probes for the sidecar; the actual upload runs
    // on the first real launch below).
    const before = await apiClient.testSSHConnection({
      name: "F5-before",
      ...(await executorTestRequest(apiClient, seedData.sshExecutorId)),
    });
    expect(before.success).toBe(true);
    expect(before.agentctl_action).toBe("needs_upload");

    // Now actually run a task so the SSH executor uploads agentctl + sidecar.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "F5 prime agentctl cache",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(
        async () => {
          const env = await apiClient.getTaskEnvironment(task.id);
          return env?.executor_type ?? null;
        },
        { message: "task should pick up the SSH executor", timeout: 60_000 },
      )
      .toBe("ssh");

    // Environment ownership is now recorded before the initial launch
    // completes. Poll the cache probe rather than treating that durable row as
    // evidence that the asynchronous SSH upload has already finished.
    await expect
      .poll(
        async () => {
          const after = await apiClient.testSSHConnection({
            name: "F5-after",
            ...(await executorTestRequest(apiClient, seedData.sshExecutorId)),
          });
          expect(after.success).toBe(true);
          return after.agentctl_action;
        },
        { message: "SSH agentctl should be cached after the initial launch", timeout: 60_000 },
      )
      .toBe("cached");

    if (!task.session_id) throw new Error("SSH cache priming task did not return a session_id");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "SSH cache priming task should finish before teardown",
    );
  });

  // F6 — WS dispatcher returns the same shape as the HTTP route. Doesn't
  // exercise the binary frames; just confirms the dispatcher routes the
  // payload through and the response body matches.
  test("ws ssh.test action returns the same shape as POST /api/v1/ssh/test", async ({
    apiClient,
    seedData,
  }) => {
    const payload = {
      name: "F6",
      ...(await executorTestRequest(apiClient, seedData.sshExecutorId)),
    };
    const http = await apiClient.testSSHConnection(payload);
    expect(http.success).toBe(true);
    expect(http.steps.length).toBeGreaterThanOrEqual(4);

    // Hit the WS dispatcher entry directly so a regression in the gateway
    // (e.g. the action handler diverging from the HTTP route) is caught
    // here, not just at the HTTP layer.
    const ws = await apiClient.wsRequest<typeof http>("ssh.test", payload);
    expect(ws.success).toBe(true);
    expect(ws.steps.map((s) => s.name)).toEqual(http.steps.map((s) => s.name));
  });
});
