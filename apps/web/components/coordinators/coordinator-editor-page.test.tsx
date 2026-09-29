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
  usePathname: () => "/settings/workspaces/w1/coordinators/c1",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mockToastError(...args), success: vi.fn() },
}));
const mockRunWithNavigationBlockerBypassed = vi.fn((fn: () => void) => fn());
vi.mock("@/lib/routing/navigation-guard", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/routing/navigation-guard")>();
  return {
    ...actual,
    runWithNavigationBlockerBypassed: (fn: () => void) => mockRunWithNavigationBlockerBypassed(fn),
  };
});

type StoreState = {
  agentProfiles: { items: AgentProfileOption[] };
  executors: { items: Executor[] };
  workspaces: { items: Array<{ id: string; scopes?: string[] }> };
  features: { coordinatorPhase2: boolean };
};

let storeState: StoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: StoreState) => unknown) => selector(storeState),
}));

import { CoordinatorEditorPage } from "./coordinator-editor-page";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";
const DELETE_BUTTON_TESTID = "delete-coordinator-button";
const COORDINATORS_LIST_PATH = "/settings/workspaces/w1/coordinators";
const SAVE_BUTTON_NAME = "Save changes";

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
    phase2?: boolean;
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
    features: { coordinatorPhase2: options.phase2 ?? false },
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
  const { rerender: rtlRerender } = render(
    <SettingsSaveProvider>
      <TooltipProvider>
        <CoordinatorEditorPage workspaceId="w1" coordinatorId="c1" />
      </TooltipProvider>
    </SettingsSaveProvider>,
  );
  const rerender = () =>
    rtlRerender(
      <SettingsSaveProvider>
        <TooltipProvider>
          <CoordinatorEditorPage workspaceId="w1" coordinatorId="c1" />
        </TooltipProvider>
      </SettingsSaveProvider>,
    );
  return { patch, remove, refresh, rerender };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("CoordinatorEditorPage: loading, errors and rendering", () => {
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
      COORDINATORS_LIST_PATH,
    );
    expect(screen.queryByLabelText("Name")).toBeNull();
  });

  it("renders the loaded coordinator's fields, All coordinators link, and both notes (AC-004.4)", () => {
    setup();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Planner");
    expect(screen.getByTestId("all-coordinators-link").getAttribute("href")).toBe(
      COORDINATORS_LIST_PATH,
    );
    expect(screen.getByText(/starts the next conversation fresh/)).toBeTruthy();
    expect(screen.getByText(/auto-approve setting is ignored/)).toBeTruthy();
  });

  it("shows profile-status warnings from the loaded coordinator (AC-005.1)", () => {
    setup({ coordinator: coordinator({ agent_profile_status: "missing" }) });
    expect(screen.getByTestId("coordinator-agent-profile-status").textContent).toContain("removed");
  });

  it("disables fields and hides Delete for a reader (AC-004.6)", () => {
    setup({ scopes: [] });
    expect((screen.getByLabelText("Name") as HTMLInputElement).disabled).toBe(true);
    expect(screen.queryByTestId(DELETE_BUTTON_TESTID)).toBeNull();
  });
});

describe("CoordinatorEditorPage: delete", () => {
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
    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith(COORDINATORS_LIST_PATH));
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

  it("deletes through the navigation-blocker bypass so a dirty form cannot block leaving a deleted coordinator (P2 regression)", async () => {
    const remove = vi.fn().mockResolvedValue(undefined);
    setup({ remove });

    // Dirty the form first: this is the scenario the bug required (a dirty
    // save contributor registers the in-app navigation blocker).
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TESTID));
    fireEvent.click(screen.getByTestId("coordinator-delete-confirm"));

    await waitFor(() => expect(remove).toHaveBeenCalled());
    expect(mockRunWithNavigationBlockerBypassed).toHaveBeenCalled();
    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith(COORDINATORS_LIST_PATH));
  });
});

describe("CoordinatorEditorPage: save", () => {
  it("saves through the settings save bar with only changed fields, then refreshes (B6, AC-004.4)", async () => {
    const patch = vi.fn().mockResolvedValue(coordinator({ name: "Renamed" }));
    const refresh = vi.fn();
    setup({ patch, refresh });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));

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
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));

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
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));

    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it("keeps an edit typed during an in-flight save instead of discarding it (P1 regression)", async () => {
    const initial = coordinator({ context: "Some context" });
    let resolvePatch: (value: Coordinator) => void = () => {};
    const patch = vi.fn(
      () =>
        new Promise<Coordinator>((resolve) => {
          resolvePatch = resolve;
        }),
    );
    const refresh = vi.fn();
    const { rerender } = setup({ coordinator: initial, patch, refresh });

    const contextField = screen.getByLabelText("Context") as HTMLTextAreaElement;
    fireEvent.change(contextField, { target: { value: "Saved value" } });
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith({ context: "Saved value" }));

    // The user keeps typing while the PATCH is still in flight.
    fireEvent.change(contextField, { target: { value: "Saved value plus more" } });

    // The save resolves. Both patch()'s own setCoordinator and the ensuing
    // refresh() replace the hook's `coordinator` reference in the real
    // implementation; simulate that by pointing the mocked hook at a fresh
    // object with only what was actually submitted, then re-render.
    const updated = coordinator({ context: "Saved value" });
    resolvePatch(updated);
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    mockUseCoordinator.mockReturnValue({
      coordinator: updated,
      status: "ready",
      refresh,
      patch,
      remove: vi.fn(),
    });
    rerender();

    expect((screen.getByLabelText("Context") as HTMLTextAreaElement).value).toBe(
      "Saved value plus more",
    );
    // The extra keystroke is still unsaved: the save bar must stay dirty.
    expect(screen.getByRole("button", { name: SAVE_BUTTON_NAME })).toBeTruthy();
  });
});

describe("CoordinatorEditorPage: phase 2 sections", () => {
  it("renders the phase-1 page with no Sections row while the flag is off", () => {
    setup();
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(screen.getByLabelText("Name")).toBeTruthy();
  });

  it("shows Identity, Standing orders and Goal with the phase-1 fields under Identity when on", () => {
    setup({ phase2: true });
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Identity",
      "Standing orders",
      "Goal",
    ]);
    expect(screen.getByLabelText("Name")).toBeTruthy();
  });
});
