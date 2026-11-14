import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { IntegratedControlProvider } from "./integrated-control-provider";
import { IntegratedControlDashboard } from "./integrated-control-dashboards";

const api = vi.hoisted(() => ({ workspaces: vi.fn(), coordinators: vi.fn(), aggregate: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listWorkspaces: api.workspaces,
}));
vi.mock("@/lib/api/domains/coordinator-api", () => ({ listCoordinators: api.coordinators }));
vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getWorkspaceAggregate: api.aggregate,
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));

const row = (id: string, name: string, extra: Record<string, number>) => ({
  workspace_id: id,
  name,
  task_count: 0,
  open_tasks: 0,
  in_progress_tasks: 0,
  blocked_tasks: 0,
  done_tasks: 0,
  pending_approvals: 0,
  agent_count: 1,
  running_agents: 0,
  ...extra,
});
const activity = (id: string) => ({
  id,
  workspace_id: "alpha",
  actor_type: "user",
  actor_id: "user-1",
  action: "task.created",
  target_type: "task",
  target_id: "task-1",
  details: "{}",
  created_at: "2026-10-20T00:00:00Z",
});

beforeEach(() => {
  api.workspaces.mockReset().mockResolvedValue({
    workspaces: [
      { id: "alpha", name: "Alpha", scopes: ["workspace.read"] },
      { id: "beta", name: "Beta", scopes: ["workspace.read"] },
      { id: "plain", name: "Plain", scopes: ["workspace.read"] },
    ],
  });
  api.coordinators.mockReset().mockResolvedValue({ coordinators: [] });
  api.aggregate.mockReset().mockResolvedValue({
    workspaces: [
      row("alpha", "Alpha", { task_count: 3, in_progress_tasks: 2, blocked_tasks: 1 }),
      row("beta", "Beta", {
        task_count: 1,
        in_progress_tasks: 1,
        running_agents: 1,
        pending_approvals: 2,
      }),
    ],
    recent_activity: [activity("a1"), activity("a2"), activity("a3")],
  });
});
afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

function renderView(view: "overview" | "history", office: boolean) {
  window.history.replaceState({}, "", `/control/${view}`);
  return render(
    <StateProvider
      initialState={{
        features: { ...defaultFeatureFlags, integratedControl: true, office },
      }}
    >
      <IntegratedControlProvider>
        <IntegratedControlDashboard view={view} />
      </IntegratedControlProvider>
    </StateProvider>,
  );
}

it("shows the stall band, attention list and comparison from one aggregate read", async () => {
  renderView("overview", true);
  const band = await screen.findByTestId("control-stall-band");
  expect(band.getAttribute("data-stalled")).toBe("1");
  expect(api.aggregate).toHaveBeenCalledTimes(1);
  const kinds = screen
    .getAllByTestId("control-attention-item")
    .map((item) => [item.getAttribute("data-kind"), item.textContent]);
  expect(kinds.map(([kind]) => kind)).toEqual(["stalled", "blocked", "approvals"]);
  expect(kinds[0][1]).toContain("Alpha");
  const rows = screen.getAllByTestId("control-compare-row");
  expect(rows.map((item) => item.getAttribute("data-status"))).toEqual(["stalled", "active"]);
  expect(within(rows[0]).getByRole("link", { name: "Alpha" }).getAttribute("href")).toBe(
    "/office?workspaceId=alpha",
  );
  expect(screen.getByTestId("control-compare-total").textContent).toContain("4");
  expect(screen.getByTestId("control-aggregate-uncovered").textContent).toContain("1");
});

it("folds repeated activity into one history row", async () => {
  renderView("history", true);
  const groups = await screen.findAllByTestId("control-history-group");
  expect(groups).toHaveLength(1);
  expect(groups[0].getAttribute("data-count")).toBe("3");
  expect(within(groups[0]).getByTestId("control-history-group-count").textContent).toContain("3");
});

it("does not read the aggregate when Office is off", async () => {
  renderView("overview", false);
  await waitFor(() => expect(api.workspaces).toHaveBeenCalled());
  expect(screen.queryByTestId("control-aggregate-overview")).toBeNull();
  expect(screen.queryByTestId("control-aggregate-state")).toBeNull();
  expect(api.aggregate).not.toHaveBeenCalled();
});

it("reports a failed first read without hiding the rest of the dashboard", async () => {
  api.aggregate.mockRejectedValue(new Error("down"));
  renderView("overview", true);
  const state = await screen.findByTestId("control-aggregate-state");
  await waitFor(() => expect(state.getAttribute("role")).toBe("alert"));
  expect(screen.getAllByText("Alpha").length).toBeGreaterThan(0);
});
