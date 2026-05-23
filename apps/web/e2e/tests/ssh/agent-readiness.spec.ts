import { test, expect } from "../../fixtures/ssh-test-base";
import { execInContainer, remotePathExists } from "../../helpers/ssh";

/**
 * The agent-readiness probe SSHs into the remote and runs `command -v` for
 * each enabled agent's first command-line token. The sshd e2e image bakes
 * `mock-agent` into /usr/local/bin/, so the default probe should report it
 * as installed. Move it aside and the second probe must flip to "missing"
 * AND carry the agent's InstallScript() through as an install hint.
 *
 * Drives the HTTP API, the shell-detection endpoint, the install endpoint,
 * and the UI card mounted on /settings/executors/[profileId].
 */
test.describe("ssh executor — agent readiness", () => {
  test("probes the remote and reports installed agents (mock-agent baked in)", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const resp = await apiClient.probeSSHAgents(seedData.sshExecutorId);
    expect(resp.host).toBe(seedData.sshTarget.host);
    expect(resp.rows.length).toBeGreaterThanOrEqual(1);

    const mock = resp.rows.find((r) => r.agent_id === "mock-agent");
    expect(mock).toBeDefined();
    expect(mock!.available).toBe(true);
    expect(mock!.binary).toBe("mock-agent");
    expect(mock!.resolved_at).toMatch(/\/usr\/local\/bin\/mock-agent$/);
  });

  test("missing binary flips status to 'missing' and carries the install hint", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    execInContainer(seedData.sshTarget, [
      "sh",
      "-c",
      "mv /usr/local/bin/mock-agent /usr/local/bin/mock-agent.bak",
    ]);
    try {
      const resp = await apiClient.probeSSHAgents(seedData.sshExecutorId);
      const mock = resp.rows.find((r) => r.agent_id === "mock-agent");
      expect(mock).toBeDefined();
      expect(mock!.available).toBe(false);
      // MockAgent's InstallScript is a deterministic echo-only command so
      // tests can assert the hint flows through end-to-end. Real agents'
      // hints (e.g. `npm install -g …`) would never run during this test
      // — the panel only displays them.
      expect(mock!.install_hint).toMatch(/mock-install/);
    } finally {
      execInContainer(seedData.sshTarget, [
        "sh",
        "-c",
        "mv /usr/local/bin/mock-agent.bak /usr/local/bin/mock-agent",
      ]);
    }
  });

  test("probe-shells returns the shells available on the remote", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const resp = await apiClient.probeSSHShells(seedData.sshExecutorId);
    expect(resp.host).toBe(seedData.sshTarget.host);
    // Alpine ships /bin/sh always; bash is installed via the kandev-sshd
    // image's apk list. zsh/fish/dash aren't installed so they should NOT
    // appear — pin the subset.
    expect(resp.available).toEqual(expect.arrayContaining(["bash", "sh"]));
    expect(resp.available).not.toContain("zsh");
    expect(resp.available).not.toContain("fish");
  });

  test("install-agent runs the agent's InstallScript and reports success", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    // MockAgent.InstallScript is `echo mock-install: step 1 && echo mock-install: step 2 && echo mock-install: done` —
    // deterministic and side-effect-free. Asserting the output mentions the
    // step labels proves the script actually ran via login shell, not that
    // we stubbed the path.
    const resp = await apiClient.installSSHAgent(seedData.sshExecutorId, {
      agent_id: "mock-agent",
    });
    expect(resp.success).toBe(true);
    expect(resp.output).toMatch(/mock-install: step 1/);
    expect(resp.output).toMatch(/mock-install: done/);
  });

  test("install-agent rejects unknown agent_id with a clean error", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(30_000);
    const rawResp = await apiClient.rawRequest(
      "POST",
      `/api/v1/ssh/executors/${seedData.sshExecutorId}/install-agent`,
      { agent_id: "does-not-exist" },
    );
    expect(rawResp.status).toBe(404);
    const body = await rawResp.json();
    expect(body.error).toMatch(/not found/i);
  });

  test("UI card renders on profile-edit page; probe + install round-trip works", async ({
    apiClient: _apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    // Land on the profile-edit page (the new home for the readiness card).
    await testPage.goto(`/settings/executors/${seedData.sshExecutorProfileId}`);
    const card = testPage.getByTestId("ssh-agent-readiness-card");
    await expect(card).toBeVisible({ timeout: 5_000 });

    // Shell selector renders + the detected-shells probe populated it.
    const shellSelector = testPage.getByTestId("ssh-readiness-shell");
    await expect(shellSelector).toBeVisible();

    // Probe the host; mock-agent should land as installed.
    await testPage.getByTestId("ssh-agent-readiness-probe").click();
    const table = testPage.getByTestId("ssh-agent-readiness-table");
    await expect(table).toBeVisible({ timeout: 30_000 });
    const mockRow = testPage.getByTestId("ssh-readiness-row-mock-agent");
    await expect(mockRow).toHaveAttribute("data-available", "true");

    // Sanity: install endpoint is reachable via the UI button when an
    // agent is missing. Move mock-agent aside, re-probe, click Install,
    // assert the row flips back to installed (the install script is a
    // no-op echo so the row stays missing in reality — but the install
    // CALL itself should succeed and the toast surfaces).
    execInContainer(seedData.sshTarget, [
      "sh",
      "-c",
      "mv /usr/local/bin/mock-agent /usr/local/bin/mock-agent.bak",
    ]);
    try {
      await testPage.getByTestId("ssh-agent-readiness-probe").click();
      await expect(mockRow).toHaveAttribute("data-available", "false", { timeout: 30_000 });
      const installBtn = testPage.getByTestId("ssh-readiness-install-mock-agent");
      await expect(installBtn).toBeVisible();
      await installBtn.click();
      // After the install call returns, the card auto re-probes. The
      // mock-agent install script is echo-only so the binary is still
      // moved aside — assertion is that the install request itself
      // succeeded (output panel appears) and didn't hang.
      const outputRow = testPage.getByTestId("ssh-readiness-row-mock-agent-output");
      await expect(outputRow).toBeVisible({ timeout: 30_000 });
      await expect(outputRow).toContainText("mock-install");
    } finally {
      // Restore so other tests in the same worker see the agent as
      // installed.
      if (remotePathExists(seedData.sshTarget, "/usr/local/bin/mock-agent.bak")) {
        execInContainer(seedData.sshTarget, [
          "sh",
          "-c",
          "mv /usr/local/bin/mock-agent.bak /usr/local/bin/mock-agent",
        ]);
      }
    }
  });
});
