import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { WorkflowStep } from "@/lib/types/http";
import { SessionConfigEditor, SessionConfigToggle } from "./workflow-session-config-editor";

const CODEX_AGENT = "codex";
const GROK_AGENT = "grok";
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
          available_models: [{ id: MODEL_ID, name: "5.6 Sol" }],
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
    ],
  }),
}));

afterEach(cleanup);

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
