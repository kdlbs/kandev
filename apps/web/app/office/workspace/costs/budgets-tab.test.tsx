import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { BudgetsTab } from "./budgets-tab";

const workspaceId = "ws-budget-proof";
const policyId = "owned-policy";
// Producer shape from the qualified policy-only HTTP response. No spend field.
const policyResponse = {
  budgets: [
    {
      id: policyId,
      workspace_id: workspaceId,
      scope_type: "workspace",
      scope_id: workspaceId,
      limit_subcents: 100000,
      period: "total",
      alert_threshold_pct: 80,
      action_on_exceed: "notify_only",
      created_at: "2026-10-14T10:18:52.642235441Z",
      updated_at: "2026-10-14T10:18:52.642235441Z",
      revision: 1,
    },
  ],
};

const requests: { path: string; method: string }[] = [];

beforeEach(() => {
  requests.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      const method = init?.method ?? "GET";
      requests.push({ path, method });
      if (method === "DELETE" && path === `/api/v1/office/budgets/${policyId}`) {
        return new Response(null, { status: 204 });
      }
      let body: unknown;
      if (path === `/api/v1/office/workspaces/${workspaceId}/budgets`) {
        body = policyResponse;
      } else if (path === `/api/v1/office/workspaces/${workspaceId}/budgets/default`) {
        body = { limit_subcents: 500000 };
      }
      if (!body || method !== "GET") throw new Error(`Unexpected HTTP request: ${method} ${path}`);
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function renderPolicy() {
  render(
    <TooltipProvider>
      <BudgetsTab workspaceId={workspaceId} />
    </TooltipProvider>,
  );
  const title = await screen.findByText(`workspace: ${workspaceId}`);
  const card = title.closest<HTMLElement>('[data-slot="card"]');
  if (!card) throw new Error("Budget card was not rendered");
  return card;
}

// @covers AC-OFFICE-COSTS-001.9
describe("first-party budget policy list", () => {
  it("does not claim measured usage from the policy-only HTTP response", async () => {
    const card = await renderPolicy();
    const policy = within(card);

    expect(policy.queryByText("Healthy")).toBeNull();
    expect(policy.getByText("Unknown")).toBeTruthy();
    expect(policy.getByText("Unavailable")).toBeTruthy();
    expect(policy.getByText("Remaining: Unavailable")).toBeTruthy();
    expect(policy.queryByText(/\$0\.00|\(0%\)|Remaining: \$10\.00/)).toBeNull();
    expect(card.querySelector(".bg-green-500, .bg-yellow-500, .bg-red-500")).toBeNull();
    expect(requests).toContainEqual({
      path: `/api/v1/office/workspaces/${workspaceId}/budgets`,
      method: "GET",
    });
  });

  it("retains normalized configuration and deletes through the real API", async () => {
    const card = await renderPolicy();
    const policy = within(card);

    expect(policy.getByText("Budget: $10.00")).toBeTruthy();
    expect(policy.getByText("Period: total")).toBeTruthy();
    expect(policy.getByText("Alert: 80%")).toBeTruthy();
    expect(policy.getByText("Action: notify only")).toBeTruthy();
    fireEvent.click(policy.getByRole("button"));
    await waitFor(() => expect(screen.queryByText(`workspace: ${workspaceId}`)).toBeNull());
    expect(requests).toContainEqual({
      path: `/api/v1/office/budgets/${policyId}`,
      method: "DELETE",
    });
    expect(
      screen.getByText("No budget policies configured. Add one to enforce spending limits."),
    ).toBeTruthy();
  });
});
