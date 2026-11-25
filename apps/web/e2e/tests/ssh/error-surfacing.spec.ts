import { test, expect } from "../../fixtures/ssh-test-base";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { execInContainer } from "../../helpers/ssh";

import {
  createSSHRoute,
  observeSSHRoute,
  trustAndTestSSHRoute,
  readSSHRoute,
} from "../../helpers/ssh-managed-fixture";

test.describe("SSH connection errors", () => {
  test("unreachable destination reports a connection failure", async ({ backend, seedData }) => {
    const route = await createSSHRoute(backend.baseUrl, "Unreachable", {
      ...seedData.sshTarget,
      port: 1,
    });
    await expect(observeSSHRoute(backend.baseUrl, route)).rejects.toThrow(/ssh_connection_failed/);
  });

  test("the destination identifies a rejected key", async ({ backend, seedData }) => {
    const key = path.join(fs.mkdtempSync(path.join(backend.tmpDir, "unknown-key-")), "identity");
    execFileSync("ssh-keygen", ["-t", "ed25519", "-f", key, "-N", "", "-q"]);
    const route = await createSSHRoute(
      backend.baseUrl,
      "Unrecognized identity",
      seedData.sshTarget,
      {
        identity: { kind: "file", reference: key },
      },
    );
    await expect(trustAndTestSSHRoute(backend.baseUrl, route)).rejects.toThrow(
      /ssh_authentication_rejected/,
    );
  });

  test("executor test reports the supported remote platform", async ({
    backend,
    apiClient,
    seedData,
  }) => {
    const executor = await apiClient.getExecutor(seedData.sshExecutorId);
    const route = await readSSHRoute(backend.baseUrl, executor.config!.ssh_connection_id);
    const result = await apiClient.testSSHConnection({
      name: "Platform control",
      ssh_connection_id: route.destination.id,
      expected_stamp: route.stamp,
    });
    expect(result.success).toBe(true);
    const platform = result.steps.find((s) => s.name === "Verify platform");
    expect(platform?.success).toBe(true);
    expect(platform?.output).toBe("linux/amd64");
  });

  test("missing agent binary on remote — launch fails with install hint, not raw exec error", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // The sshd image bakes mock-agent at /usr/local/bin/mock-agent (so
    // launches normally find it via $PATH). Move it aside to simulate a
    // fresh host that hasn't had the agent CLI installed yet, then assert
    // the pre-flight surfaces a friendly, actionable error.
    execInContainer(seedData.sshTarget, [
      "sh",
      "-c",
      "mv /usr/local/bin/mock-agent /usr/local/bin/mock-agent.bak",
    ]);
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Q-missing-binary",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.sshExecutorProfileId,
        },
      );
      // Wait for the session to settle into FAILED. We can't use
      // waitForLatestSessionDone because its terminal-set is COMPLETED/
      // WAITING_FOR_INPUT; failures land in FAILED. Poll explicitly.
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return sessions[0]?.state ?? "";
          },
          { timeout: 60_000, message: "session reaches FAILED" },
        )
        .toBe("FAILED");
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const errMsg = sessions[0]?.error_message ?? "";
      // The structured pre-flight message names the agent, the binary, and
      // the install hint sourced from agent.InstallScript(). Bare exec
      // errors (`exec: "...": executable file not found in $PATH`) used to
      // surface here verbatim — this assertion guards against regressing
      // back to that path.
      expect(errMsg).toMatch(/Mock Agent/i);
      expect(errMsg).toContain('"mock-agent"');
      expect(errMsg).toMatch(/\$PATH/);
      // Mock agent's InstallScript is a deterministic echo-only command;
      // assert at least the literal "mock-install" leaks through so we know
      // the hint block landed.
      expect(errMsg).toMatch(/mock-install/);
    } finally {
      execInContainer(seedData.sshTarget, [
        "sh",
        "-c",
        "mv /usr/local/bin/mock-agent.bak /usr/local/bin/mock-agent",
      ]);
    }
  });

  test("malformed executor test request returns a client error", async ({ apiClient }) => {
    // POST with a body that triggers a 400 from the gin route. Asserts the
    // client surfaces the HTTP error verbatim rather than synthesising a
    // SSHTestResult-shaped success=false.
    const res = await apiClient.rawRequest("POST", "/api/v1/ssh/test", "garbage");
    expect(res.status).toBe(400);
    const body = await res.text();
    expect(body).toMatch(/invalid request body|json/i);
  });
});
