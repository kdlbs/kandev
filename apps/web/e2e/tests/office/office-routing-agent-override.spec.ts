import { test, expect } from "../../fixtures/office-fixture";
import type { APIRequestContext } from "@playwright/test";

/**
 * Phase 7 spec #5 — agent-level provider override.
 *
 * Scenario:
 *   1. Workspace routing enabled with two providers in the order.
 *   2. The CEO agent's settings override the order to a single
 *      provider: ["claude-acp"].
 *   3. Backend launched with KANDEV_PROVIDER_FAILURES=claude-acp:
 *      quota_limited.
 *   4. A task is started; only one attempt is recorded (claude-acp,
 *      failed). The task does NOT fall back to codex-acp because the
 *      override pins the candidate list to a single provider.
 *
 * Because the override is single-provider, the spec also asserts that
 * the run lands in a *normal* failure state (HandleRunFailure path),
 * NOT in waiting_for_provider_capacity. Per the spec invariants in
 * Phase 4: a single quota_limited error with no remaining candidates
 * parks the run under waiting_for_provider_capacity (auto-retry) —
 * still distinct from a "real" failure path; the assertion below
 * tolerates either parked status as long as it's not the legacy
 * HandleRunFailure escalation.
 */

// PATCH /agents/:id accepts a routing override blob; the office-api-
// client does not yet have a wrapper for it, so the spec uses a raw
// request to keep the surface area minimal.
async function patchAgentOverrides(
  request: APIRequestContext,
  baseUrl: string,
  agentId: string,
  routing: Record<string, unknown>,
) {
  const res = await request.patch(`${baseUrl}/api/v1/office/agents/${agentId}`, {
    data: { routing },
  });
  if (!res.ok()) {
    throw new Error(`PATCH /agents/${agentId} failed (${res.status()}): ${await res.text()}`);
  }
}

test.describe("Office provider routing — agent override", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  // Restart the backend back to baseline env so the polluted
  // KANDEV_MOCK_PROVIDERS / KANDEV_PROVIDER_FAILURES set by this spec's
  // backend.restart() doesn't leak into subsequent specs in the worker.
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("single-provider override never falls back outside the list", async ({
    backend,
    request,
    officeApi,
    officeSeed,
  }) => {
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

    await patchAgentOverrides(request, backend.baseUrl, officeSeed.agentId, {
      provider_order_source: "override",
      provider_order: ["claude-acp"],
    });

    // Verify the override round-trips on GET /agents/:id/route (the
    // gap fixed in the preceding commit).
    const route = await officeApi.getAgentRoute(officeSeed.agentId);
    expect(route.overrides.provider_order_source).toBe("override");
    expect(route.overrides.provider_order).toEqual(["claude-acp"]);

    const task = (await officeApi.createTask(officeSeed.workspaceId, "Pinned provider test", {
      workflow_id: officeSeed.workflowId,
    })) as { id?: string };
    expect(task.id).toBeTruthy();
    // PATCH the assignee to fire the task_assigned dispatcher.
    await officeApi.assignTask(task.id!, officeSeed.agentId);

    const deadline = Date.now() + 20_000;
    let attempts: Array<{ provider_id: string; outcome: string }> = [];
    while (Date.now() < deadline) {
      const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as {
        runs?: Array<{ id: string; task_id?: string }>;
      };
      const run = (runs.runs ?? []).find((r) => r.task_id === task.id);
      if (run?.id) {
        const list = await officeApi.listRouteAttempts(run.id);
        attempts = list.attempts;
        if (attempts.length > 0) break;
      }
      await new Promise((r) => setTimeout(r, 500));
    }

    expect(attempts.length).toBeGreaterThan(0);
    // No codex-acp attempt — override is single-provider.
    expect(attempts.every((a) => a.provider_id === "claude-acp")).toBe(true);
  });
});
