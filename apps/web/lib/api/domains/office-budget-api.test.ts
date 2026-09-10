import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { createBudget, listBudgets, updateBudget } from "./office-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();
const WORKSPACE_ID = "workspace-1";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

const camelBudget = {
  scopeType: "workspace" as const,
  scopeId: WORKSPACE_ID,
  limitSubcents: 123400,
  period: "daily" as const,
  alertThresholdPct: 80,
  actionOnExceed: "block_new_tasks" as const,
};

const snakeBudget = {
  id: "budget-1",
  workspace_id: WORKSPACE_ID,
  scope_type: "workspace",
  scope_id: WORKSPACE_ID,
  limit_subcents: 123400,
  period: "daily",
  alert_threshold_pct: 80,
  action_on_exceed: "block_new_tasks",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("budget policy api", () => {
  it("maps camelCase fields to the backend snake_case create body", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ budget: snakeBudget }, 201));

    const result = await createBudget(WORKSPACE_ID, camelBudget);

    expect(lastCall().init?.method).toBe("POST");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      scope_type: "workspace",
      scope_id: WORKSPACE_ID,
      limit_subcents: 123400,
      period: "daily",
      alert_threshold_pct: 80,
      action_on_exceed: "block_new_tasks",
    });
    expect(result.scopeType).toBe("workspace");
    expect(result.limitSubcents).toBe(123400);
  });

  it("maps a partial camelCase patch to snake_case", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ budget: snakeBudget }));

    await updateBudget("budget-1", { limitSubcents: 200000, actionOnExceed: "notify_only" });

    expect(lastCall().init?.method).toBe("PATCH");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      limit_subcents: 200000,
      action_on_exceed: "notify_only",
    });
  });

  it("normalizes snake_case list responses for the web model", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ budgets: [snakeBudget] }));

    const result = await listBudgets(WORKSPACE_ID);

    expect(result.budgets).toEqual([
      expect.objectContaining({
        id: "budget-1",
        workspaceId: WORKSPACE_ID,
        scopeType: "workspace",
        scopeId: WORKSPACE_ID,
        limitSubcents: 123400,
        alertThresholdPct: 80,
        actionOnExceed: "block_new_tasks",
      }),
    ]);
  });
});
