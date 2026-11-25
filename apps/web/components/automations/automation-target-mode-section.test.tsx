import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormState } from "./automation-payload";
import { TargetModeSection } from "./automation-target-mode-section";

vi.mock("./managed-conversation-destination-selector", () => ({
  ManagedConversationDestinationSelector: () => <div data-testid="managed-destination" />,
}));

afterEach(cleanup);

function form(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "Nightly",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "automation_run",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Run",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
    continuationPolicy: "new_task",
    followUpPrompt: "",
    prContinuationStepId: "step-review",
    prMergedAction: "keep",
    prClosedAction: "keep",
    retryPolicy: {
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    },
    pluginTools: [],
    ...overrides,
  };
}

function renderSection(current: FormState) {
  const updateField = vi.fn();
  render(
    <TooltipProvider>
      <TargetModeSection
        form={current}
        savedForm={current}
        workspaceId="ws-1"
        updateField={updateField}
      />
    </TooltipProvider>,
  );
  return updateField;
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.3
describe("TargetModeSection", () => {
  it("offers the three destinations and describes the selected one", () => {
    renderSection(form());
    expect(screen.getByRole("radiogroup", { name: "Run destination" })).toBeTruthy();
    expect(
      screen.getByRole("radio", { name: "Automation history" }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(screen.getByRole("radio", { name: "Normal task" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Managed conversation" })).toBeTruthy();
    expect(
      screen.getByText(
        "Each run gets a private task that does not appear in Kanban or the sidebar.",
      ),
    ).toBeTruthy();
  });

  it("clears the per-PR continuation step when leaving normal tasks", () => {
    const updateField = renderSection(form({ taskMode: "normal_task" }));
    fireEvent.click(screen.getByRole("radio", { name: "Automation history" }));
    expect(updateField).toHaveBeenCalledWith("taskMode", "automation_run");
    expect(updateField).toHaveBeenCalledWith("prContinuationStepId", "");
  });

  it("returns repeat runs to a new task when switching to a managed conversation", () => {
    const updateField = renderSection(
      form({
        taskMode: "normal_task",
        continuationPolicy: "reuse_per_pr",
        followUpPrompt: "Review the new commits",
        prMergedAction: "archive",
        prClosedAction: "delete",
      }),
    );
    fireEvent.click(screen.getByRole("radio", { name: "Managed conversation" }));
    expect(updateField).toHaveBeenCalledWith("taskMode", "managed_conversation");
    expect(updateField).toHaveBeenCalledWith("continuationPolicy", "new_task");
    expect(updateField).toHaveBeenCalledWith("followUpPrompt", "");
    expect(updateField).toHaveBeenCalledWith("prContinuationStepId", "");
    expect(updateField).toHaveBeenCalledWith("prMergedAction", "keep");
    expect(updateField).toHaveBeenCalledWith("prClosedAction", "keep");
  });

  it("asks for a plugin conversation only for managed conversations", () => {
    renderSection(form());
    expect(screen.queryByTestId("managed-destination")).toBeNull();
    cleanup();
    renderSection(form({ taskMode: "managed_conversation" }));
    expect(screen.getByTestId("managed-destination")).toBeTruthy();
  });
});
