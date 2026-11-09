import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { IntegratedControlProvider } from "./integrated-control-provider";
import { IntegratedControlPage } from "./integrated-control-page";

const api = vi.hoisted(() => ({
  workspaces: vi.fn(),
  coordinators: vi.fn(),
  aggregate: vi.fn(),
  native: vi.fn(),
}));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listWorkspaces: api.workspaces,
}));
vi.mock("@/lib/api/domains/coordinator-api", () => ({ listCoordinators: api.coordinators }));
vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getWorkspaceAggregate: api.aggregate,
}));
vi.mock("@/lib/api/domains/control-native-api", () => ({ readNativeDashboard: api.native }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));

const workspace = { id: "alpha", name: "Alpha", scopes: ["workspace.read"] };
const secondWorkspace = { id: "beta", name: "Beta", scopes: ["workspace.read"] };
const nativeDashboard = {
  workspace_id: "alpha",
  generated_at: "2026-11-09T12:00:00Z",
  since: "2026-11-08T12:00:00Z",
  basis: "recorded_task_session_queue_transition_state",
  inference_state: "unknown",
  tasks: { IN_PROGRESS: 1 },
  sessions: {},
  waiting_input_tasks: 0,
  queued_messages: 0,
  transitions_24h: 0,
  completed_tasks_24h: 0,
  task_stages: [],
};

beforeEach(() => {
  window.history.replaceState({}, "", "/control/overview?workspace=alpha");
  api.workspaces.mockReset().mockResolvedValue({ workspaces: [workspace, secondWorkspace] });
  api.coordinators.mockReset().mockResolvedValue({ coordinators: [] });
  api.aggregate.mockReset().mockResolvedValue({ workspaces: [], recent_activity: [] });
  api.native.mockReset().mockImplementation(async (workspaceId: string) => ({
    ...nativeDashboard,
    workspace_id: workspaceId,
  }));
});

afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

function renderPage() {
  return render(
    <StateProvider
      initialState={{
        features: { ...defaultFeatureFlags, integratedControl: true, office: true },
      }}
    >
      <IntegratedControlProvider>
        <IntegratedControlPage />
      </IntegratedControlProvider>
    </StateProvider>,
  );
}

it("opens the selected workspace dashboard when a workspace has no coordinator", async () => {
  renderPage();

  await waitFor(() => expect(screen.getByTestId("control-native-comparison")).toBeTruthy());
  expect(screen.getByTestId("control-selected-workspace").textContent).toBe("Alpha");
  const tasks = screen.getByTestId("control-native-tasks");
  expect(within(tasks).getAllByTestId("control-native-tasks-row")).toHaveLength(1);
  expect(within(tasks).queryByRole("link", { name: /^Beta$/ })).toBeNull();
  expect(
    within(tasks)
      .getByRole("link", { name: /^Alpha$/ })
      .getAttribute("href"),
  ).toBe("/control/overview?workspace=alpha");
  expect(
    screen.queryByText("This conversation is unavailable or no longer accessible."),
  ).toBeNull();
});

it("keeps an unknown workspace selection unavailable", async () => {
  window.history.replaceState({}, "", "/control/overview?workspace=not-visible");
  renderPage();

  expect(
    await screen.findByText("This conversation is unavailable or no longer accessible."),
  ).toBeTruthy();
  expect(screen.queryByTestId("control-native-comparison")).toBeNull();
});
