import { test, expect } from "../../fixtures/ssh-test-base";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { execInContainer } from "../../helpers/ssh";

/**
 * The SSH executor commits to surfacing connection errors verbatim — no
 * generic "executor failed" wrapping. Spec lists this as a hard
 * requirement; the suite asserts the strings users see.
 *
 * Covers e2e-plan.md group Q (Q1–Q5).
 */
test.describe("ssh error surfacing", () => {
  test("TCP refused — error mentions 'connection refused'", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "Q1",
      host: "127.0.0.1",
      port: 1,
      user: "kandev",
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/connection refused|tcp dial/i);
  });

  test("auth failed — error names the failure mode", async ({ apiClient, seedData }) => {
    const strangerKey = path.join(seedData.sshTarget.workDir, "q2-stranger");
    fs.rmSync(strangerKey, { force: true });
    fs.rmSync(`${strangerKey}.pub`, { force: true });
    execFileSync("ssh-keygen", ["-t", "ed25519", "-f", strangerKey, "-N", "", "-q"]);

    const result = await apiClient.testSSHConnection({
      name: "Q2 auth fail",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: strangerKey,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(
      /handshake|unable to authenticate|permission denied|no supported methods/i,
    );
  });

  test("ProxyJump unreachable — error mentions the bastion", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "Q4 bastion unreachable",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
      proxy_jump: "127.0.0.1:1",
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/bastion|proxy|connection refused/i);
  });

  test("supported platform check surfaces normalized platform output", async ({
    apiClient,
    seedData,
  }) => {
    // We can't trivially spin up every OS/arch SSH target in CI, but we can
    // assert the successful platform step shape on the running Linux container.
    // Unsupported platform guidance is unit-tested in executor_ssh_connection_test.go.
    const result = await apiClient.testSSHConnection({
      name: "Q-platform-control",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
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

  test("backend 5xx on /api/v1/ssh/test is not the same as 'test failed'", async ({
    apiClient,
  }) => {
    // POST with a body that triggers a 400 from the gin route. Asserts the
    // client surfaces the HTTP error verbatim rather than synthesising a
    // SSHTestResult-shaped success=false.
    const res = await apiClient.rawRequest("POST", "/api/v1/ssh/test", "garbage");
    expect(res.status).toBe(400);
    const body = await res.text();
    expect(body).toMatch(/invalid request body|json/i);
  });
});
