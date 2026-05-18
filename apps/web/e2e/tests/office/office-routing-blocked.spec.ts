import { test, expect } from "../../fixtures/office-fixture";

/**
 * Phase 7 spec #3 — every provider blocked on user-actionable code.
 *
 * Scenario:
 *   1. Routing enabled with two providers in the order.
 *   2. Backend launched with KANDEV_PROVIDER_FAILURES injecting an
 *      auth_required / missing_credentials code per provider (both are
 *      UserAction=true, AutoRetryable=false in the classifier).
 *   3. Dispatcher records failed attempts for every candidate, sets
 *      run.routing_blocked_status = "blocked_provider_action_required".
 *   4. Inbox surfaces a `provider_degraded` item per affected provider.
 *
 * Same KANDEV_E2E_MOCK / provider-catalogue blocker as the fallback
 * spec — the routing config validator only accepts the v1 allow-list
 * provider IDs, none of which are registered when KANDEV_MOCK_AGENT=
 * only.
 */
test.describe("Office provider routing — blocked", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  // Restart back to baseline env so KANDEV_MOCK_PROVIDERS /
  // KANDEV_PROVIDER_FAILURES don't leak into sibling specs.
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("user-action codes on every provider park run as blocked", async ({
    backend,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(90_000);
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp,opencode-acp",
      KANDEV_PROVIDER_FAILURES: "claude-acp:auth_required,codex-acp:missing_credentials",
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

    const task = (await officeApi.createTask(officeSeed.workspaceId, "Blocked test", {
      workflow_id: officeSeed.workflowId,
    })) as { id?: string };
    expect(task.id).toBeTruthy();
    // PATCH the assignee to fire the task_assigned dispatcher.
    await officeApi.assignTask(task.id!, officeSeed.agentId);

    // Wait for the dispatcher to record both failed attempts and park.
    // 40s tolerates heavy parallel-suite load — the dispatcher walks
    // each provider in turn and only flips the blocked_status after
    // every routable provider has parked.
    const deadline = Date.now() + 40_000;
    let blockedStatus = "";
    while (Date.now() < deadline) {
      const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as {
        runs?: Array<{ id: string; task_id?: string; routing_blocked_status?: string }>;
      };
      const run = (runs.runs ?? []).find((r) => r.task_id === task.id);
      blockedStatus = run?.routing_blocked_status ?? "";
      if (blockedStatus === "blocked_provider_action_required") break;
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(blockedStatus).toBe("blocked_provider_action_required");

    const inbox = (await officeApi.getInbox(officeSeed.workspaceId)) as {
      items?: Array<{ type?: string; provider_id?: string }>;
    };
    const items = inbox.items ?? [];
    expect(items.some((i) => i.type === "provider_degraded")).toBe(true);

    const health = await officeApi.listRoutingHealth(officeSeed.workspaceId);
    const states = new Set(health.health.map((h) => h.provider_id));
    expect(states.has("claude-acp")).toBe(true);
    expect(states.has("codex-acp")).toBe(true);
  });
});
