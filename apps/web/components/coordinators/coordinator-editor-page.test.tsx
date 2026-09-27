import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const mockUseCoordinator = vi.fn();
const mockPush = vi.fn();
const mockReplace = vi.fn();
const mockToastError = vi.fn();

vi.mock("@/hooks/domains/settings/use-coordinator", () => ({
  useCoordinator: (...args: unknown[]) => mockUseCoordinator(...args),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
}));
vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mockToastError(...args), success: vi.fn() },
}));

type StoreState = {
  agentProfiles: { items: AgentProfileOption[] };
  executors: { items: Executor[] };
  workspaces: { items: Array<{ id: string; scopes?: string[] }> };
};

let storeState: StoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: StoreState) => unknown) => selector(storeState),
}));

import { CoordinatorEditorPage } from "./coordinator-editor-page";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";
const DELETE_BUTTON_TESTID = "delete-coordinator-button";

function mkExecutor(): Executor {
  return {
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
  };
}

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "Planner",
    agent_profile_id: "agent-1",
    executor_profile_id: "profile-1",
    context: "Some context",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

function setup(
  options: {
    status?: "loading" | "ready" | "not-found" | "error";
    coordinator?: Coordinator | null;
    scopes?: string[];
    patch?: ReturnType<typeof vi.fn>;
    remove?: ReturnType<typeof vi.fn>;
    refresh?: ReturnType<typeof vi.fn>;
  } = {},
) {
  storeState = {
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
    executors: { items: [mkExecutor()] },
    workspaces: { items: [{ id: "w1", scopes: options.scopes ?? ["workspace.manage"] }] },
  };
  const status = options.status ?? "ready";
  const coordinatorValue = options.coordinator === undefined ? coordinator() : options.coordinator;
  const patch = options.patch ?? vi.fn().mockResolvedValue(coordinatorValue ?? coordinator());
  const remove = options.remove ?? vi.fn().mockResolvedValue(undefined);
  const refresh = options.refresh ?? vi.fn();
  mockUseCoordinator.mockReturnValue({
    coordinator: coordinatorValue,
    status,
    refresh,
    patch,
    remove,
  });
  render(
    <SettingsSaveProvider>
      <TooltipProvider>
        <CoordinatorEditorPage workspaceId="w1" coordinatorId="c1" />
      </TooltipProvider>
    </SettingsSaveProvider>,
  );
  return { patch, remove, refresh };
}

describe("CoordinatorEditorPage", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows a loading state (B3)", () => {
    setup({ status: "loading", coordinator: null });
    expect(screen.getByTestId("coordinator-editor-loading")).toBeTruthy();
  });

  it("shows an inline error with Retry on a non-404 load failure (B3)", () => {
    const refresh = vi.fn();
    setup({ status: "error", coordinator: null, refresh });
    expect(screen.getByTestId("coordinator-editor-load-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refresh).toHaveBeenCalled();
  });

  it("shows not-found with an All coordinators link and no fields (B3)", () => {
    setup({ status: "not-found", coordinator: null });
    expect(screen.getByTestId("coordinator-not-found")).toBeTruthy();
    expect(screen.getByTestId("all-coordinators-link").getAttribute("href")).toBe(
      "/settings/workspaces/w1/coordinators",
    );
    expect(screen.queryByLabelText("Name")).toBeNull();
  });

  it("renders the loaded coordinator's fields, All coordinators link, and both notes (AC-004.4)", () => {
    setup();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Planner");
    expect(screen.getByTestId("all-coordinators-link").getAttribute("href")).toBe(
      "/settings/workspaces/w1/coordinators",
    );
    expect(screen.getByText(/starts the next conversation fresh/)).toBeTruthy();
    expect(screen.getByText(/auto-approve setting is ignored/)).toBeTruthy();
  });

  it("shows profile-status warnings from the loaded coordinator (AC-005.1)", () => {
    setup({ coordinator: coordinator({ agent_profile_status: "missing" }) });
    expect(screen.getByTestId("coordinator-agent-profile-status").textContent).toContain("removed");
  });

  it("shows the Delete coordinator button for a manager and opens the confirm dialog", () => {
    setup();
    expect(screen.getByTestId(DELETE_BUTTON_TESTID)).toBeTruthy();
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TESTID));
    expect(screen.getByTestId("coordinator-delete-confirm-dialog")).toBeTruthy();
  });

  it("deletes and navigates to the list on confirm (B7, AC-004.5)", async () => {
    const remove = vi.fn().mockResolvedValue(undefined);
    setup({ remove });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TESTID));
    fireEvent.click(screen.getByTestId("coordinator-delete-confirm"));

    await waitFor(() => expect(remove).toHaveBeenCalled());
    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/settings/workspaces/w1/coordinators"),
    );
  });

  it("keeps the dialog open and toasts on a delete failure, without navigating (B7)", async () => {
    const remove = vi.fn().mockRejectedValue(new Error("boom"));
    setup({ remove });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TESTID));
    fireEvent.click(screen.getByTestId("coordinator-delete-confirm"));

    await waitFor(() => expect(remove).toHaveBeenCalled());
    expect(mockReplace).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalled();
    expect(screen.getByTestId("coordinator-delete-confirm-dialog")).toBeTruthy();
  });

  it("disables fields and hides Delete for a reader (AC-004.6)", () => {
    setup({ scopes: [] });
    expect((screen.getByLabelText("Name") as HTMLInputElement).disabled).toBe(true);
    expect(screen.queryByTestId(DELETE_BUTTON_TESTID)).toBeNull();
  });

  it("saves through the settings save bar with only changed fields, then refreshes (B6, AC-004.4)", async () => {
    const patch = vi.fn().mockResolvedValue(coordinator({ name: "Renamed" }));
    const refresh = vi.fn();
    setup({ patch, refresh });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(patch).toHaveBeenCalledWith({ name: "Renamed" }));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it("shows a field error under the named field on a 400 save failure and keeps the save bar dirty (B6, B11)", async () => {
    const patch = vi
      .fn()
      .mockRejectedValue(
        new ApiError("bad request", 400, { error: "Name too long", field: "name" }),
      );
    setup({ patch });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(screen.getByTestId("coordinator-name-error").textContent).toBe("Name too long"),
    );
    expect(screen.getByRole("button", { name: "Retry save" })).toBeTruthy();
  });

  it("refreshes (to pick up the not-found state) on a 404 save failure (B6)", async () => {
    const patch = vi.fn().mockRejectedValue(new ApiError("not found", 404, {}));
    const refresh = vi.fn();
    setup({ patch, refresh });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });
});
