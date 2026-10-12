import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const mockUseCoordinators = vi.fn();
const mockPush = vi.fn();

vi.mock("@/hooks/domains/settings/use-coordinators", () => ({
  useCoordinators: (...args: unknown[]) => mockUseCoordinators(...args),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockPush }),
}));

type StoreState = {
  features: { coordinatorPhase2: boolean };
  agentProfiles: { items: AgentProfileOption[] };
  executors: { items: Executor[] };
  workspaces: { items: Array<{ id: string; scopes?: string[] }> };
};

let storeState: StoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: StoreState) => unknown) => selector(storeState),
}));

import { CoordinatorsListPage } from "./coordinators-list-page";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "Planner",
    agent_profile_id: "agent-1",
    executor_profile_id: "profile-1",
    context: "Watches the board",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

function setup(
  overrides: {
    coordinators?: Partial<ReturnType<typeof mockUseCoordinators>>;
    scopes?: string[];
    phase2?: boolean;
  } = {},
) {
  storeState = {
    features: { coordinatorPhase2: overrides.phase2 ?? false },
    agentProfiles: {
      items: [
        {
          id: "agent-1",
          label: "Claude",
          agent_id: "claude",
          agent_name: "Claude",
          cli_passthrough: false,
        },
      ],
    },
    executors: {
      items: [
        {
          id: "exec-1",
          name: "Local",
          type: "local",
          status: "ready",
          is_system: false,
          profiles: [
            {
              id: "profile-1",
              executor_id: "exec-1",
              name: "worktree",
              prepare_script: "",
              cleanup_script: "",
              created_at: FIXTURE_TIMESTAMP,
              updated_at: FIXTURE_TIMESTAMP,
            },
          ],
          created_at: FIXTURE_TIMESTAMP,
          updated_at: FIXTURE_TIMESTAMP,
        },
      ],
    },
    workspaces: { items: [{ id: "w1", scopes: overrides.scopes ?? ["workspace.manage"] }] },
  };
  mockUseCoordinators.mockReturnValue({
    items: [],
    loaded: true,
    loading: false,
    loadError: false,
    refresh: vi.fn(),
    ...overrides.coordinators,
  });
  return render(
    <TooltipProvider>
      <CoordinatorsListPage workspaceId="w1" />
    </TooltipProvider>,
  );
}

describe("CoordinatorsListPage", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows a loading state on the initial fetch (B3)", () => {
    setup({ coordinators: { loaded: false, loading: true } });
    expect(screen.getByTestId("coordinators-loading")).toBeTruthy();
  });

  it("shows an inline error with Retry when the load fails (B3)", () => {
    const refresh = vi.fn();
    setup({ coordinators: { loaded: false, loading: false, loadError: true, refresh } });
    expect(screen.getByTestId("coordinators-load-error")).toBeTruthy();
    fireEvent.click(screen.getByTestId("coordinators-retry-button"));
    expect(refresh).toHaveBeenCalled();
  });

  it("shows the empty state and an Add coordinator button for a manager (B2)", () => {
    setup();
    expect(screen.getByTestId("coordinators-empty-state")).toBeTruthy();
    expect(screen.getByTestId("add-coordinator-button")).toBeTruthy();
  });

  it("hides the Add coordinator button for a reader (AC-004.6/B10)", () => {
    setup({ scopes: [] });
    expect(screen.queryByTestId("add-coordinator-button")).toBeNull();
    expect(screen.getByTestId("coordinators-empty-state")).toBeTruthy();
  });

  it("navigates to the add page when Add coordinator is clicked", () => {
    setup();
    fireEvent.click(screen.getByTestId("add-coordinator-button"));
    expect(mockPush).toHaveBeenCalledWith("/settings/workspaces/w1/coordinators/new");
  });

  it("renders one card per coordinator with resolved agent/executor names (AC-004.2, B9)", () => {
    setup({ coordinators: { items: [coordinator()] } });
    const card = screen.getByTestId("coordinator-card-c1");
    expect(card.textContent).toContain("Planner");
    expect(card.textContent).toContain("Claude");
    expect(card.textContent).toContain("worktree");
    expect(screen.getByTestId("coordinator-open-c1").getAttribute("href")).toBe(
      "/workspaces/w1/coordinator/c1",
    );
    expect(screen.getByTestId("coordinator-configure-c1").getAttribute("href")).toBe(
      "/settings/workspaces/w1/coordinators/c1",
    );
  });

  it("shows a missing-profile label when the coordinator's profile id no longer resolves (B9)", () => {
    setup({
      coordinators: {
        items: [coordinator({ agent_profile_id: "gone", executor_profile_id: "gone-too" })],
      },
    });
    expect(screen.getByTestId("coordinator-card-c1").textContent).toContain("Removed");
  });

  it("shows the later-phase note", () => {
    setup();
    expect(screen.getByTestId("coordinators-later-phase-note")).toBeTruthy();
  });
});

describe("CoordinatorsListPage summary line", () => {
  afterEach(cleanup);

  it("renders the summary line and hides the later-phase note while phase 2 is on", () => {
    setup({
      phase2: true,
      coordinators: {
        items: [
          coordinator({
            summary: {
              watch_scope: "selected",
              watched_count: 1,
              approval_actions: 2,
              active_orders: 1,
            },
          }),
        ],
      },
    });
    expect(screen.getByTestId("coordinator-summary-c1").textContent).toBe(
      "1 board \u00b7 2 actions need approval \u00b7 1 standing order",
    );
    expect(screen.queryByTestId("coordinators-later-phase-note")).toBeNull();
  });

  it("names every board and a watch of no board", () => {
    setup({
      phase2: true,
      coordinators: {
        items: [
          coordinator({
            id: "a",
            summary: {
              watch_scope: "all",
              watched_count: 0,
              approval_actions: 1,
              active_orders: 0,
            },
          }),
          coordinator({
            id: "b",
            summary: {
              watch_scope: "selected",
              watched_count: 0,
              approval_actions: 0,
              active_orders: 2,
            },
          }),
        ],
      },
    });
    expect(screen.getByTestId("coordinator-summary-a").textContent).toBe(
      "Every board \u00b7 1 action needs approval \u00b7 0 standing orders",
    );
    expect(screen.getByTestId("coordinator-summary-b").textContent).toBe(
      "Watches no board \u00b7 0 actions need approval \u00b7 2 standing orders",
    );
  });

  it("renders no summary line when the list carries none", () => {
    setup({ phase2: true, coordinators: { items: [coordinator()] } });
    expect(screen.queryByTestId("coordinator-summary-c1")).toBeNull();
  });
});
