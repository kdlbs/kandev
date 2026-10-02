import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const patch = vi.fn();

vi.mock("@/hooks/domains/settings/use-coordinator", () => ({
  useCoordinator: () => ({
    coordinator: {
      id: "c1",
      workspace_id: "w1",
      name: "Planner",
      agent_profile_id: "agent-1",
      executor_profile_id: "profile-1",
      task_agent_profile_id: "agent-1",
      task_executor_profile_id: "profile-1",
      context: "Some context",
    },
    status: "ready",
    refresh: vi.fn(),
    patch,
    remove: vi.fn(),
  }),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/settings/workspaces/w1/coordinators/c1",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
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
            profiles: [{ id: "profile-1", executor_id: "exec-1", name: "worktree" }],
          },
        ],
      },
      workspaces: { items: [{ id: "w1", scopes: ["workspace.manage"] }] },
      features: { coordinator: true, coordinatorPhase2: true, coordinatorPhase3: true },
    }),
}));
vi.mock("./sections/coordinator-sections", () => ({
  CoordinatorSections: ({
    identity,
    onContextApplied,
  }: {
    identity: React.ReactNode;
    onContextApplied?: (context: string) => void;
  }) => (
    <div>
      {identity}
      <button type="button" onClick={() => onContextApplied?.("Applied context")}>
        simulate-apply
      </button>
    </div>
  ),
}));

import { CoordinatorEditorPage } from "./coordinator-editor-page";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("CoordinatorEditorPage: applied context", () => {
  it("replaces the context in form and baseline but keeps unsaved name edits", async () => {
    render(
      <SettingsSaveProvider>
        <TooltipProvider>
          <CoordinatorEditorPage workspaceId="w1" coordinatorId="c1" />
        </TooltipProvider>
      </SettingsSaveProvider>,
    );
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Renamed" } });
    fireEvent.click(screen.getByText("simulate-apply"));
    await waitFor(() =>
      expect((screen.getByLabelText("Context") as HTMLTextAreaElement).value).toBe(
        "Applied context",
      ),
    );
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Renamed");
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(patch).toHaveBeenCalledTimes(1));
    expect(patch).toHaveBeenCalledWith({ name: "Renamed" });
  });
});
