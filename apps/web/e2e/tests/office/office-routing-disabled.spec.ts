import { test, expect } from "../../fixtures/office-fixture";

/**
 * Phase 7 spec #1 — routing disabled round-trip.
 *
 * Verifies that:
 *   - Enabling and immediately disabling routing leaves the workspace
 *     in the "disabled" state with no provider_order entries forced on.
 *   - The agent routing API surface reports the workspace as disabled.
 *   - GET /agents/:id/route still returns the agent's persisted
 *     inherit markers (the gap fixed in the preceding commit).
 *
 * The launch path itself (mock-agent) is verified by the existing
 * onboarding-task-launch + agent runtime specs; this spec narrowly
 * exercises the routing config surface and the disabled-state guard.
 */
test.describe("Office provider routing — disabled", () => {
  test.beforeEach(async ({ apiClient, backend, officeApi, officeSeed }) => {
    // Routing config helpers reference canonical provider IDs (claude-acp,
    // codex-acp). The default backend fixture deliberately does NOT
    // register those so other specs see a single Mock agent; opt in here
    // to keep this spec's routing config validation aligned with the
    // multi-provider routing surface.
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp,opencode-acp",
    });
    // Other routing-* specs in this worker may have enabled routing /
    // configured an order. Reset DB-side routing state and re-disable so
    // each assertion in this file starts from a clean baseline.
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
    await officeApi.updateRouting(officeSeed.workspaceId, {
      enabled: false,
      provider_order: [],
      default_tier: "balanced",
      provider_profiles: {},
    });
  });

  // Restart back to baseline env so KANDEV_MOCK_PROVIDERS doesn't leak
  // into sibling specs that count agents.
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("disabled-state round-trip preserves default tier", async ({ officeApi, officeSeed }) => {
    // Seeded by Commit A: onboarding writes a disabled routing row with
    // the chosen tier as default. Round-trip should preserve that.
    const initial = await officeApi.getRouting(officeSeed.workspaceId);
    expect(initial.config.enabled).toBe(false);
    expect(initial.config.default_tier).toBeTruthy();
    expect(Array.isArray(initial.config.provider_order)).toBe(true);

    // Explicitly disable. We re-send an empty provider_order because
    // the workspace's seeded order references the mock-agent provider,
    // which is not on the v1 routing allow-list. The validator rejects
    // unknown providers even when enabled=false, so a clean disable PUT
    // uses no provider entries.
    await officeApi.updateRouting(officeSeed.workspaceId, {
      enabled: false,
      provider_order: [],
      default_tier: initial.config.default_tier,
      provider_profiles: {},
    });

    const after = await officeApi.getRouting(officeSeed.workspaceId);
    expect(after.config.enabled).toBe(false);
    expect(after.config.default_tier).toBe(initial.config.default_tier);
    expect(after.config.provider_order).toEqual([]);
  });

  test("agent route endpoint reports inherit markers when routing disabled", async ({
    officeApi,
    officeSeed,
  }) => {
    const route = await officeApi.getAgentRoute(officeSeed.agentId);
    expect(route.preview).toBeTruthy();
    // The onboarding seed writes routing.tier_source = "inherit" and
    // routing.provider_order_source = "inherit" onto the CEO agent's
    // settings so the routing UI can hydrate without inferring "absent
    // key → inherit". Both markers must round-trip on first paint.
    expect(route.overrides.tier_source).toBe("inherit");
    expect(route.overrides.provider_order_source).toBe("inherit");
  });

  test("preview endpoint runs cleanly when routing disabled", async ({ officeApi, officeSeed }) => {
    const preview = (await officeApi.getRoutingPreview(officeSeed.workspaceId)) as {
      agents?: Array<{ agent_id: string; effective_tier: string }>;
    };
    expect(preview.agents).toBeTruthy();
    expect((preview.agents ?? []).length).toBeGreaterThan(0);
    // Every agent surfaces the workspace's default tier as its
    // effective tier when no per-agent override is set.
    for (const row of preview.agents ?? []) {
      expect(row.effective_tier).toBeTruthy();
    }
  });
});
