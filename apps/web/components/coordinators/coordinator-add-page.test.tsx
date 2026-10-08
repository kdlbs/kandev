import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const mockUseCoordinators = vi.fn();
const mockPush = vi.fn();
const mockReplace = vi.fn();
const mockToastError = vi.fn();

vi.mock("@/hooks/domains/settings/use-coordinators", () => ({
  useCoordinators: (...args: unknown[]) => mockUseCoordinators(...args),
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
  workspaces: {
    items: Array<{
      id: string;
      default_agent_profile_id?: string | null;
      default_executor_id?: string | null;
      scopes?: string[];
    }>;
  };
};

let storeState: StoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: StoreState) => unknown) => selector(storeState),
}));

import { CoordinatorAddPage } from "./coordinator-add-page";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";
const ADD_BUTTON_TESTID = "add-coordinator-submit";

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
    id: "new-id",
    workspace_id: "w1",
    name: "Planner",
    agent_profile_id: "agent-1",
    executor_profile_id: "profile-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

function setup(
  options: {
    defaultAgentProfileId?: string | null;
    defaultExecutorId?: string | null;
    create?: ReturnType<typeof vi.fn>;
    scopes?: string[];
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
        {
          id: "agent-2",
          label: "Passthrough",
          agent_id: "cli",
          agent_name: "CLI",
          cli_passthrough: true,
        },
      ],
    },
    executors: { items: [mkExecutor()] },
    workspaces: {
      items: [
        {
          id: "w1",
          default_agent_profile_id: options.defaultAgentProfileId,
          default_executor_id: options.defaultExecutorId,
          scopes: options.scopes ?? ["workspace.manage"],
        },
      ],
    },
  };
  const create = options.create ?? vi.fn().mockResolvedValue(coordinator());
  mockUseCoordinators.mockReturnValue({ create });
  render(
    <TooltipProvider>
      <CoordinatorAddPage workspaceId="w1" />
    </TooltipProvider>,
  );
  return { create };
}

describe("CoordinatorAddPage", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("preselects the workspace's default agent profile and executor (B4)", () => {
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1" });
    expect(screen.getByTestId("coordinator-agent-profile-picker").textContent).toContain("Claude");
    expect(screen.getByTestId("executor-profile-selector").textContent).toContain("worktree");
  });

  it("does not preselect a CLI-passthrough default agent profile (B4)", () => {
    setup({ defaultAgentProfileId: "agent-2", defaultExecutorId: "exec-1" });
    expect(screen.getByTestId("coordinator-agent-profile-picker").textContent).not.toContain(
      "Passthrough",
    );
  });

  it("disables Add until name and both profiles are set (AC-004.3)", () => {
    setup({ defaultAgentProfileId: null, defaultExecutorId: null });
    const addButton = screen.getByTestId(ADD_BUTTON_TESTID) as HTMLButtonElement;
    expect(addButton.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Planner" } });
    expect(addButton.disabled).toBe(true);
  });

  it("enables Add once name and both profiles are set, then creates and redirects (B5)", async () => {
    const create = vi.fn().mockResolvedValue(coordinator({ id: "new-id" }));
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1", create });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Planner" } });
    const addButton = screen.getByTestId(ADD_BUTTON_TESTID) as HTMLButtonElement;
    expect(addButton.disabled).toBe(false);

    fireEvent.click(addButton);

    await waitFor(() =>
      expect(create).toHaveBeenCalledWith({
        name: "Planner",
        agent_profile_id: "agent-1",
        executor_profile_id: "profile-1",
        context: "",
      }),
    );
    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/settings/workspaces/w1/coordinators/new-id"),
    );
  });

  it("disables Add while the create request is in flight (B5)", async () => {
    let resolveCreate!: (value: Coordinator) => void;
    const create = vi.fn(
      () =>
        new Promise<Coordinator>((resolve) => {
          resolveCreate = resolve;
        }),
    );
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1", create });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Planner" } });

    const addButton = screen.getByTestId(ADD_BUTTON_TESTID) as HTMLButtonElement;
    fireEvent.click(addButton);
    await waitFor(() => expect(addButton.disabled).toBe(true));

    resolveCreate(coordinator());
    await waitFor(() => expect(mockReplace).toHaveBeenCalled());
  });

  it("shows a 400 field error under the named field and keeps form values (B5, B11)", async () => {
    const create = vi
      .fn()
      .mockRejectedValue(
        new ApiError("bad request", 400, { error: "Name too long", field: "name" }),
      );
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1", create });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Planner" } });
    fireEvent.click(screen.getByTestId(ADD_BUTTON_TESTID));

    await waitFor(() =>
      expect(screen.getByTestId("coordinator-name-error").textContent).toBe("Name too long"),
    );
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Planner");
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("toasts and leaves the form usable on a non-400 create failure (network/server error)", async () => {
    const create = vi.fn().mockRejectedValue(new Error("network down"));
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1", create });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Planner" } });
    const addButton = screen.getByTestId(ADD_BUTTON_TESTID) as HTMLButtonElement;
    fireEvent.click(addButton);

    await waitFor(() => expect(create).toHaveBeenCalled());
    expect(mockToastError).toHaveBeenCalled();
    expect(mockReplace).not.toHaveBeenCalled();
    expect(addButton.disabled).toBe(false);
  });

  it("disables every field and the submit button for a reader (SEC-004 / P2 regression)", () => {
    setup({ defaultAgentProfileId: "agent-1", defaultExecutorId: "exec-1", scopes: [] });

    expect((screen.getByLabelText("Name") as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByLabelText("Context") as HTMLTextAreaElement).disabled).toBe(true);
    expect(
      (screen.getByTestId("coordinator-agent-profile-picker") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByTestId("executor-profile-selector") as HTMLButtonElement).disabled).toBe(
      true,
    );

    expect((screen.getByTestId(ADD_BUTTON_TESTID) as HTMLButtonElement).disabled).toBe(true);
  });
});
