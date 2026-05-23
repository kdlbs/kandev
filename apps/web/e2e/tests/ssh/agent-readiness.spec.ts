import { test, expect } from "../../fixtures/ssh-test-base";
import { execInContainer } from "../../helpers/ssh";

/**
 * The agent-readiness probe SSHs into the remote and runs `command -v` for
 * each enabled agent's first command-line token. The sshd e2e image bakes
 * `mock-agent` into /usr/local/bin/, so the default probe should report it
 * as installed. Move it aside and the second probe must flip to "missing"
 * AND carry the agent's InstallScript() through as an install hint.
 *
 * Drives both contracts: the HTTP API and the UI card mounted on
 * /settings/executors/ssh/:id.
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

  test("UI card renders the probe results with install hint copy buttons", async ({
    apiClient: _apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(60_000);
    await testPage.goto(`/settings/executors/ssh/${seedData.sshExecutorId}`);
    const card = testPage.getByTestId("ssh-agent-readiness-card");
    await expect(card).toBeVisible({ timeout: 5_000 });

    // Pre-probe state: empty placeholder, no rows.
    await expect(card).toContainText("Click");

    await testPage.getByTestId("ssh-agent-readiness-probe").click();
    const table = testPage.getByTestId("ssh-agent-readiness-table");
    await expect(table).toBeVisible({ timeout: 30_000 });

    const mockRow = testPage.getByTestId("ssh-readiness-row-mock-agent");
    await expect(mockRow).toHaveAttribute("data-available", "true");
    await expect(mockRow).toContainText("Installed");
    await expect(mockRow).toContainText("mock-agent");
  });
});
