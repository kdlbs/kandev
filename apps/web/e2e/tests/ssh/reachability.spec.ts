import { test, expect } from "../../fixtures/ssh-test-base";
import { dropTrafficToPort22, restoreTraffic } from "../../helpers/ssh";
import { SSHSettingsPage } from "../../pages/SSHSettingsPage";

/**
 * SSH reachability: settings-panel state transitions and non-gating launch
 * behavior against a real sshd target.
 *
 * Drives the failure threshold through repeated "Probe now" clicks rather
 * than the scheduled poller's wall-clock interval — `httpProbeReachability`
 * runs the exact same hysteresis path (`store.Observe`) an automatic pass
 * would, so this proves the same threshold logic without making the spec's
 * runtime depend on a configured interval.
 *
 * Covers AC-EXECUTORS-SSH-REACHABILITY-001.9, 001.11, 002.1, 003.1, 003.3.
 */
test.describe("ssh executor — reachability", () => {
  test("reflects reachable, then unreachable after the failure threshold, then clears on a single success", async ({
    testPage,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const page = new SSHSettingsPage(testPage);
    await page.gotoExisting(seedData.sshExecutorId);

    // A fresh probe against the live target starts reachable.
    await page.probeNow();
    await expect(page.reachabilityState).toHaveText(/reachable/i);
    await expect(page.reachabilityHost).toContainText(seedData.sshTarget.host);

    dropTrafficToPort22(seedData.sshTarget);
    try {
      // First failed probe: below the two-probe failure threshold, so the
      // panel must not have flipped yet.
      await page.probeNow();
      await expect(page.reachabilityState).toHaveText(/reachable/i);

      // Second consecutive failure crosses the threshold.
      await page.probeNow();
      await expect(page.reachabilityState).toHaveText(/unreachable/i);
      await expect(page.reachabilityHost).toContainText(seedData.sshTarget.host);
      await expect(page.reachabilityReason).toBeVisible();
    } finally {
      restoreTraffic(seedData.sshTarget);
    }

    // A single successful probe clears an unreachable state immediately;
    // no second confirming probe is required.
    await page.probeNow();
    await expect(page.reachabilityState).toHaveText(/reachable/i);
  });

  test("a launch started while the host is unreachable is attempted and fails naming the host, not refused", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    dropTrafficToPort22(seedData.sshTarget);
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Reachability: launch while unreachable",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.sshExecutorProfileId,
        },
      );

      // The task and its session are created rather than refused — the
      // launch is attempted and only fails once the real connection attempt
      // gives up. `dropTrafficToPort22` drops inbound packets on the
      // container's own port 22, but Docker's own port-publishing plumbing
      // (docker-proxy on some hosts) can still complete the client's TCP
      // handshake before its own upstream dial to the container stalls, so
      // the failure surfaces only once that stalls out — well past the
      // client-side `sshDialTimeout` (30s) alone. Give this real, variable
      // network path a wide budget rather than one tuned to a fast local
      // failure (e.g. a missing remote binary).
      let errMsg = "";
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            const session = sessions[0];
            errMsg = session?.error_message ?? "";
            return session?.state ?? "";
          },
          { timeout: 200_000, message: "session reaches FAILED against the unreachable host" },
        )
        .toBe("FAILED");
      expect(errMsg).toContain(seedData.sshTarget.host);
    } finally {
      restoreTraffic(seedData.sshTarget);
    }
  });
});
