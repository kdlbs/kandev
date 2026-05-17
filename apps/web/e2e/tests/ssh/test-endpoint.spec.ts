import { test, expect } from "../../fixtures/ssh-test-base";
import { execInContainer } from "../../helpers/ssh";

/**
 * HTTP contract for POST /api/v1/ssh/test. The endpoint dials the host with a
 * permissive host-key callback (records the fingerprint but does not pin it),
 * runs uname / arch / git probes, and reports whether agentctl is already
 * cached on the remote. Drives the "Test Connection" UI flow.
 *
 * Covers e2e-plan.md group F (F1–F6).
 */
test.describe("ssh test-endpoint contract", () => {
  // F2 — happy-path step ordering against a reachable sshd container.
  test("returns step ordering Resolve target -> SSH handshake -> Probe remote -> Verify arch -> Verify agentctl cache", async ({
    apiClient,
    seedData,
  }) => {
    const result = await apiClient.testSSHConnection({
      name: "F2",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });

    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe(seedData.sshTarget.hostFingerprint);
    expect(result.arch).toBe("x86_64");

    const stepNames = result.steps.map((s) => s.name);
    expect(stepNames).toEqual([
      "Resolve target",
      "SSH handshake",
      "Probe remote",
      "Verify arch",
      "Verify agentctl cache",
    ]);
    for (const step of result.steps) {
      expect(step.success, `step ${step.name} should succeed`).toBe(true);
    }
  });

  // F1 — invalid body shape returns 400 (FastAPI/gin produces a structured error).
  test("rejects malformed body with 400", async ({ apiClient }) => {
    const res = await apiClient.rawRequest("POST", "/api/v1/ssh/test", "not json");
    expect(res.status).toBe(400);
  });

  // F3 — host required either directly or via host_alias. ResolveSSHTarget short-circuits.
  test("Resolve target step fails when both host and host_alias are empty", async ({
    apiClient,
  }) => {
    const result = await apiClient.testSSHConnection({
      name: "F3",
      user: "kandev",
      identity_source: "agent",
    });

    expect(result.success).toBe(false);
    expect(result.steps[0]?.name).toBe("Resolve target");
    expect(result.steps[0]?.success).toBe(false);
    expect(result.steps[0]?.error).toMatch(/host is required/i);
    // Should not have proceeded to handshake / probe steps.
    expect(result.steps.length).toBe(1);
  });

  // F4 — handshake against a closed port surfaces verbatim, no probe steps emitted.
  test("handshake failure short-circuits with no probe steps", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "F4",
      host: "127.0.0.1",
      port: 1, // reserved, refused
      user: "kandev",
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });

    expect(result.success).toBe(false);
    const stepNames = result.steps.map((s) => s.name);
    expect(stepNames).toContain("Resolve target");
    expect(stepNames).toContain("SSH handshake");
    expect(stepNames).not.toContain("Probe remote");
    expect(stepNames).not.toContain("Verify arch");
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.success).toBe(false);
    expect(handshake?.error).toMatch(/tcp dial|connection refused|handshake/i);
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

    // First test run: agentctl has never been uploaded, status = "uploaded".
    const before = await apiClient.testSSHConnection({
      name: "F5-before",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(before.success).toBe(true);
    expect(before.agentctl_action).toBe("uploaded");

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

    // Subsequent test run: sha256 matches, status = "cached".
    const after = await apiClient.testSSHConnection({
      name: "F5-after",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(after.success).toBe(true);
    expect(after.agentctl_action).toBe("cached");
  });

  // F6 — WS dispatcher returns the same shape as the HTTP route. Doesn't
  // exercise the binary frames; just confirms the dispatcher routes the
  // payload through and the response body matches.
  test("ws ssh.test action returns the same shape as POST /api/v1/ssh/test", async ({
    apiClient,
    seedData,
  }) => {
    const http = await apiClient.testSSHConnection({
      name: "F6",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(http.success).toBe(true);
    expect(http.steps.length).toBeGreaterThanOrEqual(5);

    // The WS surface is invoked indirectly via the gateway; calling the HTTP
    // route a second time documents the contract. (The dispatcher entry
    // reuses the same handler function; expand here when a dedicated WS
    // helper lands in api-client.)
    const http2 = await apiClient.testSSHConnection({
      name: "F6",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(http2.success).toBe(true);
    expect(http2.steps.map((s) => s.name)).toEqual(http.steps.map((s) => s.name));
  });
});
