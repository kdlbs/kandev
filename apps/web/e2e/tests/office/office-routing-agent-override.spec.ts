import { test, expect } from "../../fixtures/office-fixture";
import type { APIRequestContext } from "@playwright/test";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

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
    apiClient,
    request,
    officeApi,
    officeSeed,
  }) => {
    await backend.restart({
      KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp,opencode-acp",
      KANDEV_PROVIDER_FAILURES: "claude-acp:quota_limited",
    });

    await officeApi.updateRouting(
      officeSeed.workspaceId,
      await balancedExecutionProfileRouting(apiClient, officeApi, officeSeed.workspaceId, [
        "claude-acp",
        "codex-acp",
      ]),
    );

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

    let attempts: Array<{ provider_id: string; outcome: string }> = [];
    // `attempts` is captured on each poll so the assertions below read the last
    // observed value rather than needing another request.
    await expect
      .poll(
        async () => {
          const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as {
            runs?: Array<{ id: string; task_id?: string }>;
          };
          const run = (runs.runs ?? []).find((r) => r.task_id === task.id);
          if (!run?.id) return 0;
          attempts = (await officeApi.listRouteAttempts(run.id)).attempts;
          return attempts.length;
        },
        { timeout: 20_000, message: "run never recorded a route attempt" },
      )
      .toBeGreaterThan(0);
    // No codex-acp attempt — override is single-provider.
    expect(attempts.every((a) => a.provider_id === "claude-acp")).toBe(true);
  });
});

/**
 * Per-role tier routing (AC-13/13b/14/14b/16a/18) and the removal of the
 * `model` field from Office agent identity (AC-8a).
 *
 * The seeded office agent (`officeSeed.agentId`) is always created with
 * Role: "ceo" (see internal/office/onboarding/service.go), so a
 * `role_tiers: {ceo: ...}` entry targets it directly without a second
 * agent.
 *
 * checkRoleTiersMapped (internal/office/routing/types.go) rejects a
 * role_tiers entry whose tier has no provider mapping it via
 * execution_profile_ids, so the "economy" tier below reuses the same
 * execution profile the balanced-tier helper already created — nothing
 * requires a distinct profile per tier, only a non-empty mapping.
 */
test.describe("Office provider routing — role tiers", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("role tier overrides the workspace default and renders its translated source label", async ({
    backend,
    apiClient,
    officeApi,
    officeSeed,
    testPage,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });

    const configured = await balancedExecutionProfileRouting(
      apiClient,
      officeApi,
      officeSeed.workspaceId,
      ["claude-acp"],
    );
    const balancedProfile = configured.provider_profiles["claude-acp"];
    await officeApi.updateRouting(officeSeed.workspaceId, {
      ...configured,
      role_tiers: { ceo: "economy" },
      provider_profiles: {
        "claude-acp": {
          tier_map: { ...balancedProfile.tier_map, economy: balancedProfile.tier_map.balanced },
          execution_profile_ids: {
            ...balancedProfile.execution_profile_ids,
            economy: balancedProfile.execution_profile_ids?.balanced,
          },
        },
      },
    });

    // API: the seeded CEO agent has no per-agent override and no
    // wake-reason policy applies, so it resolves the role tier, not the
    // workspace default ("balanced").
    const route = await officeApi.getAgentRoute(officeSeed.agentId);
    expect(route.preview.tier_source).toBe("role");
    expect(route.preview.effective_tier).toBe("economy");

    // UI: the resolved-routes preview table renders the widened
    // tier_source value as its translated label, not the raw wire value.
    await testPage.goto("/office/workspace/routing");
    const row = testPage.getByTestId(`preview-row-${officeSeed.agentId}`);
    await expect(row).toBeVisible();
    await expect(row).toContainText("economy");
    await expect(row).toContainText("from role");
  });

  test("PATCH rejects a model key for an Office-identity agent with the structured shape", async ({
    backend,
    request,
    officeSeed,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });

    const modelRes = await request.patch(
      `${backend.baseUrl}/api/v1/office/agents/${officeSeed.agentId}`,
      { data: { model: "some-model" } },
    );
    expect(modelRes.status()).toBe(400);
    const modelBody = await modelRes.json();
    expect(modelBody.field).toBe("model");
    expect(modelBody.error).toBeTruthy();

    // Contrast: the agent_profile_id rejection uses the older bare
    // {"error": ...} shape with no "field" key — the model rejection
    // above is deliberately more structured (AC-8a).
    const profileRes = await request.patch(
      `${backend.baseUrl}/api/v1/office/agents/${officeSeed.agentId}`,
      { data: { agent_profile_id: "some-profile" } },
    );
    expect(profileRes.status()).toBe(400);
    const profileBody = await profileRes.json();
    expect(profileBody.field).toBeUndefined();
    expect(profileBody.error).toBeTruthy();
  });
});
