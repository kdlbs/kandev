import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EXECUTOR_TYPE_MAP } from "@/app/settings/executors/new/[type]/executor-types";
import { OnboardingDialog } from "./onboarding-dialog";

const apiMocks = vi.hoisted(() => ({
  listAvailableAgents: vi.fn(),
  listWorkflowTemplates: vi.fn(),
}));

const actionMocks = vi.hoisted(() => ({
  listAgentsAction: vi.fn(),
  updateAgentProfileAction: vi.fn(),
}));
const toastMock = vi.hoisted(() => vi.fn());

const staleSave = vi.hoisted(() => new Error("stale settings save"));
const CHANGED_PROFILE_MODEL = "changed-model";
const DIRTY_BUTTON_NAME = "Make agent dirty";
const TEST_AGENT_MODEL_TEST_ID = "test-agent-model";

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
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
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
    agentSettings: Record<string, { formData: { model: string } }>;
    onUpdateSetting: (agentName: string, formPatch: { model: string }) => void;
  }) => (
    <>
      <button
        type="button"
        disabled={!agentSettings["test-agent"]}
        onClick={() => onUpdateSetting("test-agent", { model: CHANGED_PROFILE_MODEL })}
      >
        {DIRTY_BUTTON_NAME}
      </button>
      <output data-testid={TEST_AGENT_MODEL_TEST_ID}>
        {agentSettings["test-agent"]?.formData.model}
      </output>
    </>
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
  vi.resetAllMocks();
  coordinatorState.snapshot = { reloadRequired: false, source: null, ownerCount: 0 };
  coordinatorState.listeners.clear();
  apiMocks.listAvailableAgents.mockResolvedValue({ agents: [availableAgent], tools: [] });
  apiMocks.listWorkflowTemplates.mockResolvedValue({ templates: [] });
  actionMocks.listAgentsAction.mockResolvedValue({ agents: [savedAgent] });
  actionMocks.updateAgentProfileAction.mockResolvedValue({});
});

afterEach(cleanup);

async function openExecutorStep(onComplete = vi.fn()) {
  render(<OnboardingDialog open onComplete={onComplete} />);
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  await screen.findByText("Executors");
  return onComplete;
}

describe("OnboardingDialog executor discovery", () => {
  it("shows the supported Settings executors as non-selectable information cards", async () => {
    await openExecutorStep();

    const cards = screen.getAllByTestId(/^onboarding-executor-card-/);
    const cardIds = cards.map((card) => card.getAttribute("data-executor-id"));
    const settingsExecutorIds = Object.keys(EXECUTOR_TYPE_MAP).filter(
      (executorId) => executorId !== "remote_docker",
    );

    expect(cardIds[0]).toBe("worktree");
    expect([...cardIds].sort()).toEqual(settingsExecutorIds.sort());
    expect(screen.queryByTestId("onboarding-executor-card-remote_docker")).toBeNull();
    expect(screen.queryByTestId("onboarding-executor-card-mock_remote")).toBeNull();

    for (const card of cards) {
      expect(card.tagName).toBe("DIV");
      expect(card.getAttribute("role")).toBeNull();
      expect(card.querySelector("button, a")).toBeNull();
    }
  });

  it("explains executor boundaries, setup needs, profiles, and the executor guide", async () => {
    await openExecutorStep();

    const worktree = screen.getByTestId("onboarding-executor-card-worktree");
    const local = screen.getByTestId("onboarding-executor-card-local");
    const docker = screen.getByTestId("onboarding-executor-card-local_docker");

    expect(worktree.textContent).toContain("Recommended for existing Git repositories");
    expect(worktree.textContent).toContain("separate Git checkout");
    expect(worktree.textContent).toContain("host account");
    expect(local.textContent).toContain("selected folder");
    expect(local.textContent).toContain("Kandev host");
    expect(docker.textContent).toContain("Docker daemon");
    expect(docker.textContent).toContain("mounts");
    expect(docker.textContent).not.toMatch(/full isolation/i);
    expect(screen.getByText(/An executor chooses where work runs\./).textContent).toContain(
      "Settings > Executors",
    );
    expect(screen.getByRole("link", { name: "View executor guide" }).getAttribute("href")).toBe(
      "https://kandev.ai/docs/executors",
    );
  });

  it("keeps Back, Next, and Skip as the only wizard actions", async () => {
    const onComplete = await openExecutorStep();

    expect(screen.getByRole("button", { name: "Back" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Next" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Skip" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Worktree" })).toBeNull();
    expect(onComplete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Agentic Workflows");
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    await screen.findByText("Executors");
    expect(onComplete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Skip" }));
    expect(onComplete).toHaveBeenCalledOnce();
  });
});

describe("OnboardingDialog profile state refresh", () => {
  // @covers AC-EXECUTORS-ONBOARDING-001.9
  it("drops dirty edits when their profile was replaced while the tour was hidden", async () => {
    const replacementAgent = {
      ...savedAgent,
      profiles: [{ ...savedAgent.profiles[0], id: "profile-2", model: "fresh-model" }],
    };
    actionMocks.listAgentsAction
      .mockResolvedValueOnce({ agents: [savedAgent] })
      .mockResolvedValueOnce({ agents: [replacementAgent] });
    const page = render(<OnboardingDialog open onComplete={vi.fn()} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);

    page.rerender(<OnboardingDialog open={false} onComplete={vi.fn()} />);
    page.rerender(<OnboardingDialog open onComplete={vi.fn()} />);
    await waitFor(() => expect(actionMocks.listAgentsAction).toHaveBeenCalledTimes(2));
    await waitFor(() => {
      expect(screen.getByTestId(TEST_AGENT_MODEL_TEST_ID).textContent).toBe("fresh-model");
    });

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Executors");
    expect(actionMocks.updateAgentProfileAction).not.toHaveBeenCalled();
  });
});

describe("OnboardingDialog profile save actions", () => {
  // @covers AC-EXECUTORS-ONBOARDING-001.10
  it("sends only one profile save when Next is clicked repeatedly", async () => {
    let resolveSave!: () => void;
    actionMocks.updateAgentProfileAction.mockImplementationOnce(
      () => new Promise<void>((resolve) => (resolveSave = resolve)),
    );
    render(<OnboardingDialog open onComplete={vi.fn()} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);

    const nextButton = screen.getByRole("button", { name: "Next" }) as HTMLButtonElement;
    fireEvent.click(nextButton);
    fireEvent.click(nextButton);

    expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledTimes(1);
    expect(nextButton.disabled).toBe(true);
    await act(async () => resolveSave());
    await screen.findByText("Executors");
  });

  // @covers AC-EXECUTORS-ONBOARDING-001.10
  it("sends only one profile save when Get Started is clicked repeatedly", async () => {
    const onComplete = vi.fn();
    render(<OnboardingDialog open onComplete={onComplete} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Executors");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Agentic Workflows");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Command Panel");

    actionMocks.updateAgentProfileAction.mockClear();
    let resolveSave!: () => void;
    actionMocks.updateAgentProfileAction.mockImplementationOnce(
      () => new Promise<void>((resolve) => (resolveSave = resolve)),
    );
    const getStartedButton = screen.getByRole("button", {
      name: "Get Started",
    }) as HTMLButtonElement;
    fireEvent.click(getStartedButton);
    fireEvent.click(getStartedButton);

    expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledTimes(1);
    expect(getStartedButton.disabled).toBe(true);
    await act(async () => resolveSave());
    await waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
  });

  // @covers AC-EXECUTORS-ONBOARDING-001.10
  it("shows an error toast when profile saving fails unexpectedly", async () => {
    actionMocks.updateAgentProfileAction.mockRejectedValue(new Error("network failure"));
    render(<OnboardingDialog open onComplete={vi.fn()} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);

    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => {
      expect(toastMock).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Error",
          description: "Could not save agent profile changes. Please try again.",
          variant: "error",
        }),
      );
    });
    expect(screen.getByText("AI Agents")).toBeTruthy();
  });
});

describe("OnboardingDialog backend restart recovery", () => {
  it("keeps dirty profile edits across a hidden phone state and saves only when proceeding", async () => {
    const onComplete = vi.fn();
    const page = render(<OnboardingDialog open onComplete={onComplete} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
    })) as HTMLButtonElement;
    await waitFor(() => expect(dirtyButton.disabled).toBe(false));
    fireEvent.click(dirtyButton);
    expect(screen.getByTestId(TEST_AGENT_MODEL_TEST_ID).textContent).toBe(CHANGED_PROFILE_MODEL);
    expect(actionMocks.updateAgentProfileAction).not.toHaveBeenCalled();

    page.rerender(<OnboardingDialog open={false} onComplete={onComplete} />);
    expect(screen.queryByTestId(TEST_AGENT_MODEL_TEST_ID)).toBeNull();
    expect(actionMocks.updateAgentProfileAction).not.toHaveBeenCalled();

    page.rerender(<OnboardingDialog open onComplete={onComplete} />);
    await waitFor(() => expect(actionMocks.listAgentsAction).toHaveBeenCalledTimes(2));
    await waitFor(() => {
      expect(screen.getByTestId(TEST_AGENT_MODEL_TEST_ID).textContent).toBe(CHANGED_PROFILE_MODEL);
    });
    expect(actionMocks.updateAgentProfileAction).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => {
      expect(actionMocks.updateAgentProfileAction).toHaveBeenCalledWith(
        "profile-1",
        expect.objectContaining({ model: CHANGED_PROFILE_MODEL }),
      );
    });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it("keeps the wizard on the agent step after a handled stale save", async () => {
    actionMocks.updateAgentProfileAction.mockRejectedValue(staleSave);
    const onComplete = vi.fn();

    render(<OnboardingDialog open onComplete={onComplete} />);
    const dirtyButton = (await screen.findByRole("button", {
      name: DIRTY_BUTTON_NAME,
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
      name: DIRTY_BUTTON_NAME,
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
