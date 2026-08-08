import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import { __resetModelConfigResolutionCache } from "@/hooks/domains/settings/use-dynamic-models";
import { SettingsSaveProvider } from "./settings-save-provider";
import { persistWorkflowDraft } from "./workflow-card-actions";
import { useWorkflowDraftContributor } from "./use-workflow-draft-contributor";
import { SessionConfigEditor, SessionConfigToggle } from "./workflow-session-config-editor";

const CODEX_AGENT = "codex";
const GROK_AGENT = "grok";
const DYNAMIC_ONLY_AGENT = "dynamic-only";
const DYNAMIC_ONLY_MODEL = "dynamic-model";
const EFFORT_OPTION = "reasoning_effort";
const MODEL_ID = "gpt-5.6-sol";

vi.mock("@/hooks/domains/settings/use-healthy-agent-profiles", () => ({
  useHealthyAgentProfiles: () => [
    {
      id: "codex-profile",
      label: "Codex • Default",
      agent_id: "codex-agent",
      agent_name: CODEX_AGENT,
      cli_passthrough: false,
    },
    {
      id: "dynamic-only-profile",
      label: "Dynamic only • Default",
      agent_id: "dynamic-only-agent",
      agent_name: DYNAMIC_ONLY_AGENT,
      cli_passthrough: false,
    },
  ],
}));

vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({
    items: [
      {
        name: CODEX_AGENT,
        display_name: "Codex",
        model_config: {
          default_model: MODEL_ID,
          current_model_id: MODEL_ID,
          available_models: [
            { id: MODEL_ID, name: "5.6 Sol" },
            { id: "model-b", name: "Model B" },
          ],
          config_options: [
            {
              type: "select",
              id: EFFORT_OPTION,
              name: "Effort",
              current_value: "high",
              options: [
                { value: "high", name: "High" },
                { value: "max", name: "Max" },
              ],
            },
          ],
          supports_dynamic_models: true,
        },
      },
      {
        name: GROK_AGENT,
        display_name: "Grok",
        model_config: {
          default_model: "grok-4",
          current_model_id: "grok-4",
          available_models: [{ id: "grok-4", name: "Grok 4" }],
          config_options: [],
          supports_dynamic_models: true,
        },
      },
      {
        name: DYNAMIC_ONLY_AGENT,
        display_name: "Dynamic only",
        model_config: {
          default_model: DYNAMIC_ONLY_MODEL,
          current_model_id: DYNAMIC_ONLY_MODEL,
          available_models: [],
          config_options: [],
          supports_dynamic_models: true,
        },
      },
    ],
  }),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: vi.fn(async (_agentName: string, request: { model: string }) => ({
    agent_name: "codex",
    model: request.model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: "reasoning_effort",
        name: "Effort",
        current_value: "high",
        options: [
          { value: "high", name: "High" },
          { value: "max", name: "Max" },
        ],
      },
    ],
    error: null,
  })),
}));

vi.mock("./workflow-card-actions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./workflow-card-actions")>();
  return {
    ...actual,
    persistWorkflowDraft: vi.fn(
      async (input: { workflow: Workflow; draftSteps: WorkflowStep[] }) => ({
        workflow: input.workflow,
        steps: input.draftSteps,
      }),
    ),
  };
});

afterEach(() => {
  cleanup();
  __resetModelConfigResolutionCache();
});

beforeEach(() => {
  vi.mocked(resolveAgentModelConfig).mockReset();
  vi.mocked(resolveAgentModelConfig).mockImplementation(
    async (_agentName: string, request: { model: string }) => ({
      agent_name: CODEX_AGENT,
      model: request.model,
      status: "ok",
      config_options: [
        {
          type: "select",
          id: EFFORT_OPTION,
          name: "Effort",
          current_value: "high",
          options: [
            { value: "high", name: "High" },
            { value: "max", name: "Max" },
          ],
        },
      ],
      error: null,
    }),
  );
});

function step(id: string, position: number, events?: WorkflowStep["events"]): WorkflowStep {
  return {
    id,
    workflow_id: "workflow-1" as WorkflowStep["workflow_id"],
    name: id,
    position,
    color: "bg-muted",
    events,
    created_at: "",
    updated_at: "",
  };
}

function SessionConfigEditorHarness({
  currentStep,
  steps,
  onUpdate,
}: {
  currentStep: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
}) {
  return (
    <>
      <SessionConfigToggle step={currentStep} onUpdate={onUpdate} readOnly={false} />
      <SessionConfigEditor step={currentStep} steps={steps} onUpdate={onUpdate} readOnly={false} />
    </>
  );
}

const testWorkflow = {
  id: "workflow-1",
  workspace_id: "workspace-1",
  name: "Workflow",
  created_at: "",
  updated_at: "",
} as Workflow;

function WorkflowSaveHarness({ initialStep }: { initialStep: WorkflowStep }) {
  const [currentStep, setCurrentStep] = useState(initialStep);
  const [workflowSteps, setWorkflowSteps] = useState([initialStep]);
  const [savedWorkflowSteps, setSavedWorkflowSteps] = useState([initialStep]);
  const [isSessionConfigResolutionPending, setSessionConfigResolutionPending] = useState(false);

  useWorkflowDraftContributor({
    workflow: testWorkflow,
    isWorkflowDirty: true,
    workflowSteps,
    savedWorkflowSteps,
    setWorkflowSteps,
    setSavedWorkflowSteps,
    mutationGuard: {
      guardMutation: async ({ operation }: { operation: () => Promise<void> }) => operation(),
    } as never,
    toast: vi.fn(),
    onWorkflowSaved: vi.fn(),
    onDiscardWorkflow: vi.fn(),
    onDeleteWorkflow: async () => undefined,
    isSessionConfigResolutionPending,
  });

  const onUpdate = (updates: Partial<WorkflowStep>) => {
    setCurrentStep((current) => ({ ...current, ...updates }));
    setWorkflowSteps((steps) => steps.map((step) => ({ ...step, ...updates })));
  };

  return (
    <SessionConfigEditor
      step={currentStep}
      steps={[currentStep]}
      onUpdate={onUpdate}
      readOnly={false}
      onResolutionPendingChange={setSessionConfigResolutionPending}
    />
  );
}

function configuredStep(agentName: string, model: string): WorkflowStep {
  return step("configured-step", 0, {
    on_enter: [
      {
        type: "configure_session",
        config: { rules: [{ agent_name: agentName, operation: "set", model }] },
      },
    ],
  });
}

describe("SessionConfigEditor", () => {
  it("keeps conditional settings hidden until the header toggle is enabled", () => {
    const onUpdate = vi.fn();
    const currentStep = step("work", 0);
    const { rerender } = render(
      <SessionConfigEditorHarness
        currentStep={currentStep}
        steps={[currentStep]}
        onUpdate={onUpdate}
      />,
    );

    expect(screen.queryByTestId("work-session-config-editor")).toBeNull();
    const toggle = screen.getByLabelText("Override original session options");
    expect(toggle).toBeTruthy();

    fireEvent.click(toggle);
    const updates = onUpdate.mock.calls[0]?.[0] as Partial<WorkflowStep>;
    expect(updates.events?.on_enter?.[0]).toEqual(
      expect.objectContaining({
        type: "configure_session",
        config: {
          rules: [expect.objectContaining({ agent_name: CODEX_AGENT, operation: "set" })],
        },
      }),
    );

    const enabledStep = { ...currentStep, ...updates };
    rerender(
      <SessionConfigEditorHarness
        currentStep={enabledStep}
        steps={[enabledStep]}
        onUpdate={onUpdate}
      />,
    );
    expect(screen.getByTestId("work-session-config-editor")).toBeTruthy();
    expect(screen.getByText("Configure original settings options")).toBeTruthy();
    fireEvent.click(screen.getByTestId("session-config-agent-0"));
    expect(screen.getAllByText("Codex").length).toBeGreaterThan(0);
    expect(screen.queryByText("Grok")).toBeNull();
  });

  it("offers keep, restore, and set-new choices for carried settings", () => {
    const onUpdate = vi.fn();
    const source = step("work", 0, {
      on_enter: [
        {
          type: "configure_session",
          config: { rules: [{ agent_name: CODEX_AGENT, operation: "set", model: MODEL_ID }] },
        },
      ],
      on_turn_complete: [{ type: "move_to_next" }],
    });
    const reviewStep = step("review", 1);
    render(
      <SessionConfigEditorHarness
        currentStep={reviewStep}
        steps={[source, reviewStep]}
        onUpdate={onUpdate}
      />,
    );

    expect(screen.getByLabelText("Override original session options")).toBeTruthy();
    expect(screen.getByTestId("session-config-carry-warning")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Restore" }));
    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        events: expect.objectContaining({
          on_enter: [
            expect.objectContaining({
              type: "configure_session",
              config: {
                rules: [{ agent_name: CODEX_AGENT, operation: "restore_original" }],
              },
            }),
          ],
        }),
      }),
    );
  });

  it("disables conditional configuration while a fixed profile override is selected", () => {
    const fixedStep = { ...step("work", 0), agent_profile_id: "codex-profile" };
    render(
      <>
        <SessionConfigToggle step={fixedStep} onUpdate={vi.fn()} readOnly={false} />
        <SessionConfigEditor step={fixedStep} steps={[]} onUpdate={vi.fn()} readOnly={false} />
      </>,
    );

    expect(
      screen.getByLabelText("Override original session options").getAttribute("disabled"),
    ).not.toBeNull();
    expect(screen.queryByTestId("work-session-config-editor")).toBeNull();
  });
});

describe("SessionConfigRuleCard model options", () => {
  it("loads model-specific options in a workflow session override", async () => {
    const currentStep = step("work", 0, {
      on_enter: [
        {
          type: "configure_session",
          config: { rules: [{ agent_name: CODEX_AGENT, operation: "set", model: MODEL_ID }] },
        },
      ],
    });

    render(
      <SessionConfigEditorHarness
        currentStep={currentStep}
        steps={[currentStep]}
        onUpdate={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Settings for codex" }));
    expect(await screen.findByTestId("config-option-trigger-reasoning_effort")).toBeTruthy();
  });

  it("preserves a saved option value during the initial resolution", async () => {
    const onUpdate = vi.fn();
    const currentStep = step("work", 0, {
      on_enter: [
        {
          type: "configure_session",
          config: {
            rules: [
              {
                agent_name: CODEX_AGENT,
                operation: "set",
                model: MODEL_ID,
                config_options: { reasoning_effort: "medium" },
              },
            ],
          },
        },
      ],
    });

    render(
      <SessionConfigEditorHarness
        currentStep={currentStep}
        steps={[currentStep]}
        onUpdate={onUpdate}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Settings for codex" }));
    expect(await screen.findByTestId("config-option-trigger-reasoning_effort")).toBeTruthy();
    expect(onUpdate).not.toHaveBeenCalled();
  });
});

describe("SessionConfigRuleCard save coordination", () => {
  it("blocks the coordinated workflow save until model options finish resolving", async () => {
    let resolveSecondResponse: ((value: unknown) => void) | undefined;
    const secondResponse = new Promise((resolve) => {
      resolveSecondResponse = resolve;
    });
    vi.mocked(resolveAgentModelConfig)
      .mockResolvedValueOnce({
        agent_name: CODEX_AGENT,
        model: MODEL_ID,
        status: "ok",
        config_options: [],
        error: null,
      })
      .mockReturnValueOnce(secondResponse as never);

    const initialStep = configuredStep(CODEX_AGENT, MODEL_ID);
    render(
      <SettingsSaveProvider>
        <WorkflowSaveHarness initialStep={initialStep} />
      </SettingsSaveProvider>,
    );

    const saveButton = await screen.findByRole("button", { name: "Save changes" });
    await waitFor(() => expect(saveButton.hasAttribute("disabled")).toBe(false));

    fireEvent.click(screen.getByRole("button", { name: "Settings for codex" }));
    fireEvent.click(await screen.findByText("Model B"));
    await waitFor(() => expect(saveButton.hasAttribute("disabled")).toBe(true));

    fireEvent.click(saveButton);
    expect(persistWorkflowDraft).not.toHaveBeenCalled();

    await act(async () => {
      resolveSecondResponse?.({
        agent_name: CODEX_AGENT,
        model: "model-b",
        status: "ok",
        config_options: [],
        error: null,
      });
    });
    await waitFor(() => expect(saveButton.hasAttribute("disabled")).toBe(false));

    fireEvent.click(saveButton);
    await waitFor(() => expect(persistWorkflowDraft).toHaveBeenCalledOnce());
  });
});

describe("SessionConfigRuleCard resolution status", () => {
  it("shows loading status when a dynamic-only agent has no baseline options", async () => {
    let resolveResponse: ((value: unknown) => void) | undefined;
    const response = new Promise((resolve) => {
      resolveResponse = resolve;
    });
    vi.mocked(resolveAgentModelConfig).mockReturnValueOnce(response as never);

    render(
      <SessionConfigEditorHarness
        currentStep={configuredStep(DYNAMIC_ONLY_AGENT, DYNAMIC_ONLY_MODEL)}
        steps={[]}
        onUpdate={vi.fn()}
      />,
    );

    expect(await screen.findByTestId("model-config-resolution-loading")).toBeTruthy();
    expect(
      screen.getByText(
        "Model options are unavailable for this agent. Refresh agent capabilities before saving this condition.",
      ),
    ).toBeTruthy();

    await act(async () => {
      resolveResponse?.({
        agent_name: DYNAMIC_ONLY_AGENT,
        model: DYNAMIC_ONLY_MODEL,
        status: "failed",
        config_options: [],
        error: "provider unavailable",
      });
    });
  });

  it("shows a retryable failure and loads options after retry for a dynamic-only agent", async () => {
    vi.mocked(resolveAgentModelConfig)
      .mockResolvedValueOnce({
        agent_name: DYNAMIC_ONLY_AGENT,
        model: DYNAMIC_ONLY_MODEL,
        status: "failed",
        config_options: [],
        error: "provider unavailable",
      })
      .mockResolvedValueOnce({
        agent_name: DYNAMIC_ONLY_AGENT,
        model: DYNAMIC_ONLY_MODEL,
        status: "ok",
        config_options: [
          {
            type: "select",
            id: EFFORT_OPTION,
            name: "Effort",
            current_value: "max",
            options: [{ value: "max", name: "Max" }],
          },
        ],
        error: null,
      });

    render(
      <SessionConfigEditorHarness
        currentStep={configuredStep(DYNAMIC_ONLY_AGENT, DYNAMIC_ONLY_MODEL)}
        steps={[]}
        onUpdate={vi.fn()}
      />,
    );

    expect(await screen.findByTestId("model-config-resolution-error")).toBeTruthy();
    expect(screen.getByText("Model options could not be loaded.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(resolveAgentModelConfig).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "Settings for dynamic-only" }));
    expect(await screen.findByTestId(`config-option-trigger-${EFFORT_OPTION}`)).toBeTruthy();
  });

  it("shows an authentication retry status for a dynamic-only agent", async () => {
    vi.mocked(resolveAgentModelConfig).mockResolvedValueOnce({
      agent_name: DYNAMIC_ONLY_AGENT,
      model: DYNAMIC_ONLY_MODEL,
      status: "auth_required",
      config_options: [],
      error: "authentication required",
    });

    render(
      <SessionConfigEditorHarness
        currentStep={configuredStep(DYNAMIC_ONLY_AGENT, DYNAMIC_ONLY_MODEL)}
        steps={[]}
        onUpdate={vi.fn()}
      />,
    );

    expect(await screen.findByTestId("model-config-resolution-error")).toBeTruthy();
    expect(screen.getByText("Sign in to this agent to load model options.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
