import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormState } from "./automation-payload";
import { AgentSection, DestinationSection, InstructionsSection } from "./automation-editor-groups";

vi.mock("./automation-target-mode-section", () => ({ TargetModeSection: () => null }));
vi.mock("./config-section", () => ({
  ConfigSection: () => null,
  WorkflowField: ({ onWorkflowChange }: { onWorkflowChange: (id: string) => void }) => (
    <>
      <button type="button" onClick={() => onWorkflowChange("wf-1")}>
        Same workflow
      </button>
      <button type="button" onClick={() => onWorkflowChange("wf-2")}>
        Other workflow
      </button>
    </>
  ),
}));
vi.mock("./automation-agent-tools-section", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./automation-agent-tools-section")>()),
  AgentToolsDisclosure: () => null,
}));
vi.mock("./prompt-section", () => ({ PromptSection: () => null, PlaceholderList: () => null }));
vi.mock("./automation-continuation-section", () => ({
  ContinuationPolicySection: () => null,
  FollowUpPromptField: () => null,
}));

afterEach(cleanup);

function form(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "Nightly",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "normal_task",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Run",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
    continuationPolicy: "reuse_per_pr",
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

describe("DestinationSection", () => {
  function renderDestination() {
    const updateField = vi.fn();
    render(
      <DestinationSection
        form={form()}
        savedForm={form()}
        workspaceId="ws-1"
        updateField={updateField}
      />,
    );
    return updateField;
  }

  it("keeps the chosen steps when the same workflow is picked again", () => {
    const updateField = renderDestination();
    fireEvent.click(screen.getByRole("button", { name: "Same workflow" }));
    expect(updateField).not.toHaveBeenCalled();
  });

  it("clears the steps of the previous workflow when another is picked", () => {
    const updateField = renderDestination();
    fireEvent.click(screen.getByRole("button", { name: "Other workflow" }));
    expect(updateField).toHaveBeenCalledWith("workflowId", "wf-2");
    expect(updateField).toHaveBeenCalledWith("workflowStepId", "");
    expect(updateField).toHaveBeenCalledWith("prContinuationStepId", "");
  });
});

describe("AgentSection", () => {
  function agentCard(current: FormState, saved: FormState) {
    render(
      <AgentSection
        form={current}
        savedForm={saved}
        workspaceId="ws-1"
        supportsMultiRepo
        updateField={vi.fn()}
        mcp={{ saveFailed: false, reload: vi.fn() }}
        pluginTools={{ groups: [], loading: false, error: false, reload: vi.fn() }}
      />,
    );
    return screen
      .getByTestId("automation-agent-section")
      .querySelector("[data-settings-dirty-level=container]");
  }

  it("marks the card changed when only the tools changed", () => {
    const saved = form({ mcpSelection: { builtin_packages: ["core"], definition_ids: [] } });
    const current = form({
      mcpSelection: { builtin_packages: ["core", "rich-output"], definition_ids: [] },
    });
    expect(agentCard(current, saved)?.getAttribute("data-settings-dirty")).toBe("true");
  });

  it("treats reordered tool choices as unchanged", () => {
    const saved = form({
      mcpSelection: { builtin_packages: ["core", "rich-output"], definition_ids: [] },
    });
    const current = form({
      mcpSelection: { builtin_packages: ["rich-output", "core"], definition_ids: [] },
    });
    expect(agentCard(current, saved)?.getAttribute("data-settings-dirty")).toBe("false");
  });
});

describe("InstructionsSection", () => {
  it("labels the task title field and its help", () => {
    render(
      <InstructionsSection
        form={form()}
        savedForm={form()}
        updateField={vi.fn()}
        placeholders={[]}
        defaultTaskTitle=""
      />,
    );
    const input = screen.getByLabelText("Task title");
    expect(input.tagName).toBe("INPUT");
    const help = document.getElementById(input.getAttribute("aria-describedby") ?? "");
    expect(help?.textContent).toContain("Supports placeholders");
  });
});
