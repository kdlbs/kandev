import { test, expect } from "../../fixtures/office-fixture";

/**
 * Phase 7 spec #4 — recovery from degraded state.
 *
 * Scenario:
 *   1. Workspace routing enabled; a provider is degraded by a prior
 *      KANDEV_PROVIDER_FAILURES injection (claude-acp:quota_limited).
 *   2. The user clicks "Retry now" on the dashboard provider-health
 *      card.
 *   3. The backend is restarted with the injection cleared.
 *   4. The provider's next probe succeeds; health flips to "healthy"
 *      and the WS event provider_health_changed updates the UI without
 *      a page reload.
 *
 * Same provider-catalogue blocker as office-routing-fallback. This spec
 * is structured against the API surface so flipping the catalogue flag
 * is enough to unblock execution.
 */
test.describe("Office provider routing — recovery", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  // Restart back to baseline env so KANDEV_MOCK_PROVIDERS /
  // KANDEV_PROVIDER_FAILURES don't leak into sibling specs.
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("clearing the injection + retry returns provider to healthy", async ({
    backend,
    officeApi,
    officeSeed,
  }) => {
    // Step 1: induce a degraded state.
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp,opencode-acp",
      KANDEV_PROVIDER_FAILURES: "claude-acp:quota_limited",
    });
    await officeApi.updateRouting(officeSeed.workspaceId, {
      enabled: true,
      provider_order: ["claude-acp", "codex-acp"],
      default_tier: "balanced",
      provider_profiles: {
        "claude-acp": { tier_map: { balanced: "sonnet" }, mode: "default" },
        "codex-acp": { tier_map: { balanced: "gpt-5" }, mode: "default" },
      },
    });
    const degradeTask = (await officeApi.createTask(officeSeed.workspaceId, "Degrade trigger", {
      workflow_id: officeSeed.workflowId,
    })) as { id?: string };
    // PATCH the assignee to fire the task_assigned dispatcher.
    await officeApi.assignTask(degradeTask.id!, officeSeed.agentId);

    // Wait for claude-acp to enter degraded state.
    const degradedDeadline = Date.now() + 20_000;
    while (Date.now() < degradedDeadline) {
      const health = await officeApi.listRoutingHealth(officeSeed.workspaceId);
      if (health.health.some((h) => h.provider_id === "claude-acp" && h.state === "degraded")) {
        break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }

    // Step 2: clear injection and call /routing/retry to fast-forward
    // the recovery probe.
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp,opencode-acp",
      KANDEV_PROVIDER_FAILURES: "",
    });
    const retry = await officeApi.retryRoutingProvider(officeSeed.workspaceId, "claude-acp");
    expect(retry.status).toBeTruthy();

    // Step 3: health should flip back to healthy (or the row should
    // disappear since /routing/health only lists non-healthy rows).
    // E2E mock providers register a ProviderProber wired to the
    // injection map, so /routing/retry flips the row without needing
    // a follow-up launch.
    const healthyDeadline = Date.now() + 20_000;
    let recovered = false;
    while (Date.now() < healthyDeadline) {
      const health = await officeApi.listRoutingHealth(officeSeed.workspaceId);
      const claude = health.health.find((h) => h.provider_id === "claude-acp");
      if (!claude || claude.state === "healthy") {
        recovered = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(recovered).toBe(true);
  });
});
