import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowSelectorRow } from "./workflow-selector-row";
import type { TaskCreateLaunchPreview } from "./task-create-dialog-launch-preview";

const touchState = vi.hoisted(() => ({ enabled: false }));
const workflowApiMocks = vi.hoisted(() => ({ listWorkflowSteps: vi.fn() }));
const WORKFLOW_SELECTOR_TRIGGER = "workflow-selector-trigger";

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

vi.mock("@/lib/api/domains/workflow-api", () => workflowApiMocks);

afterEach(() => {
  touchState.enabled = false;
  workflowApiMocks.listWorkflowSteps.mockReset();
  cleanup();
});

const launchPreview: TaskCreateLaunchPreview = {
  stepId: "step-1",
  stepName: "In Progress",
  stepPrompt: "Run {{task_prompt}}",
};

function renderSelector(preview: TaskCreateLaunchPreview | null = launchPreview) {
  return render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[{ id: "workflow-1", name: "Development" }]}
        snapshots={{}}
        selectedWorkflowId="workflow-1"
        onWorkflowChange={() => {}}
        agentProfiles={[]}
        launchPreview={preview}
      />
    </TooltipProvider>,
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("WorkflowSelectorRow launch destination", () => {
  // @covers AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1
  it("shows the launch destination arrow and explanation after the selector", async () => {
    renderSelector();

    const trigger = screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER);
    const row = screen.getByTestId("workflow-selector-row");
    const launchStep = screen.getByTestId("task-create-launch-step");

    expect(trigger.textContent).toContain("Development");
    expect(trigger.contains(launchStep)).toBe(false);
    expect(row.contains(trigger)).toBe(true);
    expect(row.contains(launchStep)).toBe(true);
    expect(trigger.className).not.toContain("flex-1");
    expect(launchStep.textContent).toBe("In Progress");
    expect(launchStep.className).not.toContain("ml-auto");

    const launchStepInfo = screen.getByTestId("task-create-launch-step-info");
    expect(trigger.contains(launchStepInfo)).toBe(false);
    expect(launchStepInfo.getAttribute("aria-label")).toBe("Learn about the task start step");
    expect(screen.getByTestId("task-create-launch-step-arrow")).toBeTruthy();
    expect(Array.from(row.children).indexOf(launchStepInfo)).toBeLessThan(
      Array.from(row.children).indexOf(launchStep),
    );

    fireEvent.focus(launchStepInfo);
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      "The task starts in this workflow step. With a task description, an auto-start step can take priority over the configured Start step.",
    );
  });

  it("opens the explanation in a drawer for coarse pointers", () => {
    touchState.enabled = true;
    renderSelector();

    const launchStepInfo = screen.getByTestId("task-create-launch-step-info");
    expect(launchStepInfo.getAttribute("aria-haspopup")).toBe("dialog");
    expect(launchStepInfo.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(launchStepInfo);

    expect(launchStepInfo.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("task-create-launch-step-help-drawer").textContent).toContain(
      "The task starts in this workflow step.",
    );
  });

  it("does not show a destination when the selected workflow has no preview", () => {
    renderSelector(null);

    expect(screen.queryByTestId("task-create-launch-step")).toBeNull();
  });
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1
it("loads every option with only one cached snapshot", async () => {
  workflowApiMocks.listWorkflowSteps.mockImplementation(async (workflowId: string) => ({
    steps: [
      {
        id: `${workflowId}-step`,
        name: `${workflowId} step`,
        position: 0,
        color: "#123456",
        is_start_step: false,
      },
    ],
    total: 1,
  }));

  render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[
          { id: "feature", name: "Feature" },
          { id: "review", name: "Review" },
          { id: "kanban", name: "Kanban" },
        ]}
        snapshots={{
          kanban: {
            workflowId: "kanban",
            workflowName: "Kanban",
            steps: [
              {
                id: "stale-kanban-step",
                title: "Stale cached step",
                position: 0,
                color: "#000000",
              },
            ],
            tasks: [],
          },
        }}
        previewWorkspaceId="workspace-1"
        selectedWorkflowId="feature"
        onWorkflowChange={() => {}}
        agentProfiles={[]}
      />
    </TooltipProvider>,
  );

  fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER));

  expect(await screen.findByText("feature step")).toBeTruthy();
  expect(screen.getByText("review step")).toBeTruthy();
  expect(screen.getByText("kanban step")).toBeTruthy();
  expect(screen.queryByText("Stale cached step")).toBeNull();
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(3);
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledWith("feature", {
    cache: "no-store",
  });
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledWith("review", {
    cache: "no-store",
  });
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledWith("kanban", {
    cache: "no-store",
  });
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.2 and AC-TASKS-CREATE-WORKFLOW-STEPS-001.3
it("shows independent loading and failure states and retries without selecting", async () => {
  const failed = deferred<{
    steps: Array<{ id: string; name: string; position: number; color: string }>;
  }>();
  const retrying = deferred<{
    steps: Array<{ id: string; name: string; position: number; color: string }>;
  }>();
  let attempts = 0;
  workflowApiMocks.listWorkflowSteps.mockImplementation((workflowId: string) => {
    if (workflowId === "review") {
      attempts += 1;
      return attempts === 1 ? failed.promise : retrying.promise;
    }
    if (workflowId === "empty") {
      return Promise.resolve({ steps: [] });
    }
    return Promise.resolve({
      steps: [{ id: "feature-step", name: "Analysis", position: 0, color: "#abcdef" }],
    });
  });
  const onWorkflowChange = vi.fn();

  render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[
          { id: "feature", name: "Feature" },
          { id: "review", name: "Review" },
          { id: "empty", name: "Empty" },
        ]}
        snapshots={{}}
        previewWorkspaceId="workspace-1"
        selectedWorkflowId="feature"
        onWorkflowChange={onWorkflowChange}
        agentProfiles={[]}
      />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER));

  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(screen.getByRole("status").textContent).toContain("Review: Loading steps…");
  expect(await screen.findByText("Analysis")).toBeTruthy();
  expect(await screen.findByText("No steps in this workflow")).toBeTruthy();
  await act(async () => {
    failed.reject(new Error("private server detail"));
  });

  expect(await screen.findByText("Failed to load workflow steps")).toBeTruthy();
  expect(screen.queryByText("private server detail")).toBeNull();
  expect(screen.getAllByRole("status")).toHaveLength(1);
  const status = screen.getByRole("status");
  expect(status.closest("button")).toBeNull();
  expect(status.textContent).toContain("Review: Failed to load workflow steps");
  const workflowOption = screen.getByRole("button", { name: "Review" });
  const retry = screen.getByTestId("workflow-preview-retry-review");
  expect(retry.tagName).toBe("BUTTON");
  retry.focus();
  expect(document.activeElement).toBe(retry);

  fireEvent.click(retry);
  expect(retry.isConnected).toBe(true);
  expect(retry.getAttribute("aria-disabled")).toBe("true");
  expect(document.activeElement).toBe(retry);
  expect(workflowOption.getAttribute("aria-label")).toBe("Review");
  expect(status.textContent).toContain("Review: Loading steps");
  await act(async () => {
    retrying.resolve({
      steps: [{ id: "review-step", name: "Recovered", position: 0, color: "#123456" }],
    });
  });
  expect(await screen.findByText("Recovered")).toBeTruthy();
  expect(screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER).getAttribute("aria-expanded")).toBe("true");
  expect(document.activeElement).toBe(workflowOption);
  expect(onWorkflowChange).not.toHaveBeenCalled();
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.5
it("preserves loaded step order, colors, start markers, and agent badges", async () => {
  workflowApiMocks.listWorkflowSteps.mockResolvedValue({
    steps: [
      {
        id: "second",
        name: "Review",
        position: 1,
        color: "#ff0000",
        agent_profile_id: "agent-profile-1",
      },
      {
        id: "first",
        name: "Analysis",
        position: 0,
        color: "#00ff00",
        is_start_step: true,
      },
    ],
  });
  render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[{ id: "feature", name: "Feature" }]}
        snapshots={{}}
        previewWorkspaceId="workspace-1"
        selectedWorkflowId="feature"
        onWorkflowChange={() => {}}
        agentProfiles={[
          {
            id: "agent-profile-1",
            label: "Review Agent",
            agent_id: "agent-1",
            agent_name: "OpenCode",
            cli_passthrough: false,
          },
        ]}
      />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER));

  const option = screen.getByTestId("workflow-option-select-feature");
  expect(await screen.findByText("Analysis")).toBeTruthy();
  expect(option.textContent?.indexOf("Analysis")).toBeLessThan(
    option.textContent?.indexOf("Review") ?? 0,
  );
  expect(option.querySelector('[style*="background-color: #00ff00"]')).toBeTruthy();
  expect(option.querySelector('[style*="background-color: #ff0000"]')).toBeTruthy();
  expect(option.textContent).toContain("*");
  expect(screen.getByTestId("step-agent-logo")).toBeTruthy();
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.5
it("retains snapshot rendering for selectors without task-create scope", () => {
  render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[{ id: "automation", name: "Automation" }]}
        snapshots={{
          automation: {
            workflowId: "automation",
            workflowName: "Automation",
            steps: [
              {
                id: "snapshot-step",
                title: "Cached automation step",
                position: 0,
                color: "#123456",
              },
            ],
            tasks: [],
          },
        }}
        selectedWorkflowId="automation"
        onWorkflowChange={() => {}}
        agentProfiles={[]}
      />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR_TRIGGER));

  expect(screen.getByText("Cached automation step")).toBeTruthy();
  expect(workflowApiMocks.listWorkflowSteps).not.toHaveBeenCalled();
});
