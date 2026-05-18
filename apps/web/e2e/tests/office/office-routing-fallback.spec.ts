import { test, expect } from "../../fixtures/office-fixture";

/**
 * Phase 7 spec #2 — provider fallback under quota_limited.
 *
 * Scenario (per docs/specs/office-provider-routing/spec.md):
 *   1. Workspace routing enabled with order `claude-acp → codex-acp`,
 *      tier `balanced`.
 *   2. Backend is launched with `KANDEV_PROVIDER_FAILURES=claude-acp:
 *      quota_limited` so the classifier short-circuits to `quota_limited`
 *      for any claude-acp launch.
 *   3. A task is started; the dispatcher records a failed attempt for
 *      claude-acp (outcome=failed_provider_unavailable, error_code=
 *      quota_limited), marks provider health as degraded, then falls
 *      back to codex-acp which launches successfully.
 *   4. Run detail header surfaces the fallback reason; dashboard
 *      provider-health card shows claude-acp as degraded.
 *
 * Per-test injection is handled by restarting the backend with the
 * new env var. The `routingerr.LoadInjectionFromEnv` map is read once
 * via sync.Once at process start, so per-test changes require a
 * backend restart — `backend.restart({ KANDEV_PROVIDER_FAILURES: ... })`
 * is the supported escape hatch from the worker fixture.
 */
test.describe("Office provider routing — fallback", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    // Routing tests share a worker; the e2eReset wipes routing state
    // (provider_health, route_attempts, runs) so each test starts with
    // a clean catalogue.
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  // Restart back to baseline env so KANDEV_MOCK_PROVIDERS /
  // KANDEV_PROVIDER_FAILURES don't leak into sibling specs.
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("claude-acp quota fails over to codex-acp", async ({ backend, officeApi, officeSeed }) => {
    test.setTimeout(90_000);
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

    const task = (await officeApi.createTask(officeSeed.workspaceId, "Fallback test", {
      workflow_id: officeSeed.workflowId,
    })) as { id?: string };
    expect(task.id).toBeTruthy();
    // Office's task→run dispatcher is event-driven off task_assigned;
    // the bare createTask call leaves the task unassigned, so it never
    // hits the routing path. PATCH the assignee to fire the pipeline.
    await officeApi.assignTask(task.id!, officeSeed.agentId);

    // Poll for the run + its attempt list. The first attempt should be
    // claude-acp failed_provider_unavailable; the second codex-acp
    // launched. The scheduler claims runs within a few ticks normally,
    // but a parallel-suite run that just restarted the backend with new
    // KANDEV_MOCK_PROVIDERS needs the full agent-registry rediscovery
    // walk to complete first — 40s rides that out without affecting
    // the happy path (resolves in <5s in isolation).
    const deadline = Date.now() + 40_000;
    let attempts: Array<{ provider_id: string; outcome: string; error_code?: string }> = [];
    while (Date.now() < deadline) {
      const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as {
        runs?: Array<{ id: string; task_id?: string }>;
      };
      const run = (runs.runs ?? []).find((r) => r.task_id === task.id);
      if (run?.id) {
        const list = await officeApi.listRouteAttempts(run.id);
        attempts = list.attempts;
        if (attempts.length >= 2) break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }

    expect(attempts.length).toBeGreaterThanOrEqual(2);
    expect(attempts[0].provider_id).toBe("claude-acp");
    expect(attempts[0].outcome).toBe("failed_provider_unavailable");
    expect(attempts[0].error_code).toBe("quota_limited");
    expect(attempts[1].provider_id).toBe("codex-acp");
    expect(attempts[1].outcome).toBe("launched");

    const health = await officeApi.listRoutingHealth(officeSeed.workspaceId);
    const claudeHealth = health.health.find((h) => h.provider_id === "claude-acp");
    expect(claudeHealth?.state).toBe("degraded");
    expect(claudeHealth?.error_code).toBe("quota_limited");
  });
});
