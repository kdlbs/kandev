import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OnboardingDialog } from "./onboarding-dialog";

const apiMocks = vi.hoisted(() => ({
  listAvailableAgents: vi.fn(),
  listWorkflowTemplates: vi.fn(),
}));

const actionMocks = vi.hoisted(() => ({
  listAgentsAction: vi.fn(),
  updateAgentProfileAction: vi.fn(),
}));

const staleSave = vi.hoisted(() => new Error("stale settings save"));

const coordinatorState = vi.hoisted(() => ({
  snapshot: {
    reloadRequired: false,
    source: null as "boot_id_changed" | "settings_interlock_rejected" | null,
    ownerCount: 0,
  },
  listeners: new Set<() => void>(),
}));

vi.mock("@/lib/api", () => apiMocks);
vi.mock("@/app/actions/agents", () => actionMocks);
vi.mock("@/lib/api/client", () => ({
  isHandledApiError: (error: unknown) => error === staleSave,
}));
vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  backendReloadCoordinator: {
    getSnapshot: () => coordinatorState.snapshot,
    subscribe: (listener: () => void) => {
      coordinatorState.listeners.add(listener);
      return () => coordinatorState.listeners.delete(listener);
    },
  },
}));
vi.mock("@kandev/ui/dialog", () => {
  const Content = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return {
    Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
      open ? <div data-testid="onboarding-dialog">{children}</div> : null,
    DialogContent: Content,
    DialogHeader: Content,
    DialogTitle: Content,
    DialogFooter: Content,
    DialogDescription: Content,
  };
});
vi.mock("@/components/onboarding/step-agents", () => ({
  StepAgents: ({
    agentSettings,
    onUpdateSetting,
  }: {
    agentSettings: Record<string, unknown>;
    onUpdateSetting: (agentName: string, formPatch: { model: string }) => void;
  }) => (
    <button
      type="button"
      disabled={!agentSettings["test-agent"]}
      onClick={() => onUpdateSetting("test-agent", { model: "changed-model" })}
    >
      Make agent dirty
    </button>
  ),
}));

const availableAgent = {
  name: "test-agent",
  display_name: "Test Agent",
  model_config: {
    default_model: "default-model",
    current_mode_id: "default-mode",
    status: "ok",
    available_models: [],
  },
  permission_settings: {},
};

const savedAgent = {
  name: "test-agent",
  profiles: [
    {
      id: "profile-1",
      name: "Default",
      model: "default-model",
      mode: "default-mode",
      allowIndexing: false,
      autoApprove: false,
      cliPassthrough: false,
      cliFlags: [],
      commandPrefix: "",
    },
  ],
};

function signalReloadRequired() {
  coordinatorState.snapshot = {
    reloadRequired: true,
    source: "settings_interlock_rejected",
    ownerCount: 0,
  };
  coordinatorState.listeners.forEach((listener) => listener());
}

beforeEach(() => {
  vi.clearAllMocks();
  coordinatorState.snapshot = { reloadRequired: false, source: null, ownerCount: 0 };
  coordinatorState.listeners.clear();
  apiMocks.listAvailableAgents.mockResolvedValue({ agents: [availableAgent], tools: [] });
  apiMocks.listWorkflowTemplates.mockResolvedValue({ templates: [] });
  actionMocks.listAgentsAction.mockResolvedValue({ agents: [savedAgent] });
  actionMocks.updateAgentProfileAction.mockResolvedValue({});
});

afterEach(cleanup);

describe("OnboardingDialog backend restart recovery", () => {
  it("keeps the wizard on the agent step after a handled stale save", async () => {
    actionMocks.updateAgentProfileAction.mockRejectedValue(staleSave);
    const onComplete = vi.fn();

    render(<OnboardingDialog open onComplete={onComplete} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: "Make agent dirty",
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledOnce());
    expect(screen.getByText("AI Agents")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Next" })).toBeTruthy();
    expect(onComplete).not.toHaveBeenCalled();
  });

  it("blocks get started and yields the modal when the final save is stale", async () => {
    let saveCount = 0;
    actionMocks.updateAgentProfileAction.mockImplementation(async () => {
      saveCount += 1;
      if (saveCount === 2) {
        signalReloadRequired();
        throw staleSave;
      }
      return {};
    });
    const onComplete = vi.fn();

    render(<OnboardingDialog open onComplete={onComplete} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: "Make agent dirty",
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("Executors")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("Agentic Workflows")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => expect(screen.getByText("Command Panel")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Get Started" }));

    await waitFor(() => expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByTestId("onboarding-dialog")).toBeNull());
    expect(onComplete).not.toHaveBeenCalled();
  });
});
