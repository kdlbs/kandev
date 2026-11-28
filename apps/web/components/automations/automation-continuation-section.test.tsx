import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormState } from "./automation-payload";
import { ContinuationPolicySection } from "./automation-continuation-section";

vi.mock("@/hooks/use-workflow-steps", () => ({
  useWorkflowSteps: (workflowId: string) => ({
    steps: workflowId
      ? [
          { id: "step-start", name: "Triage" },
          { id: "step-review", name: "Review" },
        ]
      : [],
    loading: false,
  }),
}));

afterEach(cleanup);

const PER_PR = { name: "One task per pull request" };
const SHARED = { name: "One shared task" };
const STEP_SELECTOR = "pr-continuation-step";

function form(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "PR review",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "normal_task",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Review",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 2,
    continuationPolicy: "new_task",
    followUpPrompt: "",
    prContinuationStepId: "",
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

function renderSection(current: FormState, hasPRTrigger = true) {
  const updateField = vi.fn();
  render(
    <TooltipProvider>
      <ContinuationPolicySection
        form={current}
        savedForm={current}
        updateField={updateField}
        hasPRTrigger={hasPRTrigger}
      />
    </TooltipProvider>,
  );
  return updateField;
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1
describe("per-PR continuation option", () => {
  it("is offered only with a github_pr trigger or a saved per-PR policy", () => {
    renderSection(form(), false);
    expect(screen.queryByRole("radio", PER_PR)).toBeNull();
    cleanup();

    renderSection(form({ continuationPolicy: "reuse_per_pr" }), false);
    expect(screen.getByRole("radio", PER_PR)).toBeTruthy();
    cleanup();

    const updateField = renderSection(form(), true);
    fireEvent.click(screen.getByRole("radio", PER_PR));
    expect(updateField).toHaveBeenCalledWith("continuationPolicy", "reuse_per_pr");
    expect(updateField).not.toHaveBeenCalledWith("maxConcurrentRuns", expect.anything());
  });

  it("clears per-PR options when another policy is chosen", () => {
    const updateField = renderSection(
      form({
        continuationPolicy: "reuse_per_pr",
        followUpPrompt: "x",
        prContinuationStepId: "step-review",
        prMergedAction: "archive",
        prClosedAction: "delete",
      }),
    );
    fireEvent.click(screen.getByRole("radio", { name: "New task each run" }));
    expect(updateField).toHaveBeenCalledWith("continuationPolicy", "new_task");
    expect(updateField).toHaveBeenCalledWith("followUpPrompt", "");
    expect(updateField).toHaveBeenCalledWith("prContinuationStepId", "");
    expect(updateField).toHaveBeenCalledWith("prMergedAction", "keep");
    expect(updateField).toHaveBeenCalledWith("prClosedAction", "keep");
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.2
describe("continuation step selector", () => {
  it("is shown only for per-PR normal tasks and defaults to staying put", () => {
    renderSection(form({ continuationPolicy: "reuse_per_pr", taskMode: "automation_run" }));
    expect(screen.queryByTestId(STEP_SELECTOR)).toBeNull();
    cleanup();

    renderSection(form({ continuationPolicy: "new_task" }));
    expect(screen.queryByTestId(STEP_SELECTOR)).toBeNull();
    cleanup();

    const updateField = renderSection(form({ continuationPolicy: "reuse_per_pr" }));
    const selector = screen.getByTestId(STEP_SELECTOR);
    expect(selector.textContent).toBe("Stay in current step");
    fireEvent.click(selector);
    fireEvent.click(screen.getByRole("option", { name: "Review" }));
    expect(updateField).toHaveBeenCalledWith("prContinuationStepId", "step-review");
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4
describe("end-of-PR actions", () => {
  it("saves each outcome's action and warns only for delete", () => {
    const updateField = renderSection(form({ continuationPolicy: "reuse_per_pr" }));
    expect(screen.queryByText(/Deleting removes the task's conversation and files/)).toBeNull();

    fireEvent.click(screen.getByTestId("pr-merged-action"));
    fireEvent.click(screen.getByRole("option", { name: "Archive task" }));
    expect(updateField).toHaveBeenCalledWith("prMergedAction", "archive");

    fireEvent.click(screen.getByTestId("pr-closed-action"));
    fireEvent.click(screen.getByRole("option", { name: "Delete task" }));
    expect(updateField).toHaveBeenCalledWith("prClosedAction", "delete");
    cleanup();

    renderSection(form({ continuationPolicy: "reuse_per_pr", prClosedAction: "delete" }));
    expect(screen.getByText(/Deleting removes the task's conversation and files/)).toBeTruthy();
  });
});

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1
describe("task for each run wording", () => {
  it("names the choice by the task each run uses", () => {
    renderSection(form());
    expect(screen.getByRole("radiogroup", { name: "Task for each run" })).toBeTruthy();
    expect(
      screen.getByText("Each run starts with its own conversation and files. Runs can overlap."),
    ).toBeTruthy();
  });

  it("says that one shared task spans pull requests only when a PR trigger exists", () => {
    renderSection(form({ continuationPolicy: "reuse_thread" }), true);
    expect(screen.getByRole("radio", SHARED).getAttribute("aria-checked")).toBe("true");
    expect(
      screen.getByText(/different pull requests share one conversation and files/),
    ).toBeTruthy();
    cleanup();

    renderSection(form({ continuationPolicy: "reuse_thread" }), false);
    expect(screen.queryByText(/different pull requests/)).toBeNull();
    expect(screen.getByText(/Every run continues the same task/)).toBeTruthy();
  });

  it("states that different pull requests never share a per-PR task", () => {
    renderSection(form({ continuationPolicy: "reuse_per_pr" }));
    expect(screen.getByText(/Different pull requests never share a task/)).toBeTruthy();
  });
});
