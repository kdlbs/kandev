import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { moveTask } from "@/lib/api";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

const { appStoreState, moveTaskMock, previewWorkflowMoveMock } = vi.hoisted(() => ({
  moveTaskMock: vi.fn(),
  previewWorkflowMoveMock: vi.fn().mockResolvedValue(undefined),
  appStoreState: {
    connection: { status: "connected", error: null, issueSeverity: "none" },
    workspaceContextGeneration: 1,
    workflows: { items: [], activeId: null },
    tasks: { activeSessionId: null },
    chatInput: { planModeBySessionId: {} },
    kanban: {
      tasks: [
        {
          id: "task-1",
          state: "SCHEDULING",
          workflowStepId: "work",
        },
      ],
    },
    kanbanMulti: { snapshots: {} },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {} },
    agentProfiles: { items: [] },
    setPlanMode: vi.fn(),
    setActiveDocument: vi.fn(),
  },
}));

vi.mock("@/lib/api", () => ({
  moveTask: moveTaskMock,
  previewWorkflowMove: previewWorkflowMoveMock,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appStoreState) => unknown) => selector(appStoreState),
}));
vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: () => vi.fn(),
}));
vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: () => vi.fn(),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => vi.fn(),
}));
vi.mock("@/hooks/use-toolbar-collapsed", () => ({
  useToolbarCollapsed: () => false,
}));
vi.mock("./workflow-move-options", () => ({
  useWorkflowMoveOptionsForm: () => ({
    draft: {},
    patchDraft: vi.fn(),
    resetDraft: vi.fn(),
  }),
  WorkflowMoveOptionsFields: () => null,
  workflowMoveOptionsPayload: () => undefined,
}));

const STEPS: WorkflowStepperStep[] = [
  { id: "spec", name: "Spec", color: "#111", position: 0 },
  { id: "work", name: "Work", color: "#222", position: 1 },
  { id: "review", name: "Review", color: "#333", position: 2 },
];

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("WorkflowStepper full-layout keyboard disclosure", () => {
  it("focuses a real full-layout trigger and opens its existing hover card", async () => {
    const view = render(
      <WorkflowStepper
        steps={STEPS}
        currentStepId="work"
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );

    const trigger = screen.getByTestId("workflow-step-Work");
    expect(trigger.tagName).toBe("BUTTON");
    trigger.focus();
    expect(document.activeElement).toBe(trigger);

    await waitFor(
      () => {
        expect(screen.getByTestId("workflow-step-popover")).toBeTruthy();
        expect(screen.getByTestId("workflow-step-progress-work").textContent).toContain(
          "Preparing agent",
        );
      },
      { timeout: 1000 },
    );
    expect(document.activeElement).toBe(trigger);

    appStoreState.kanban.tasks[0].workflowStepId = "review";
    view.rerender(
      <WorkflowStepper
        steps={STEPS}
        currentStepId="review"
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );
    const destinationTrigger = screen.getByTestId("workflow-step-Review");
    destinationTrigger.focus();
    await waitFor(() => {
      expect(destinationTrigger.getAttribute("aria-current")).toBe("step");
      expect(screen.getByTestId("workflow-step-progress-review").textContent).toContain(
        "Preparing agent",
      );
    });
  });

  it("keeps the full disclosure open while focusing and activating Move here", async () => {
    moveTaskMock.mockResolvedValue({});
    render(
      <WorkflowStepper
        steps={STEPS}
        currentStepId="work"
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );

    const destinationTrigger = screen.getByTestId("workflow-step-Review");
    destinationTrigger.focus();
    const moveButton = await screen.findByTestId("workflow-step-move-here");

    moveButton.focus();
    expect(document.activeElement).toBe(moveButton);
    fireEvent.keyDown(moveButton, { key: "Enter", code: "Enter" });
    fireEvent.keyUp(moveButton, { key: "Enter", code: "Enter" });
    moveButton.click();

    await waitFor(() =>
      expect(moveTask).toHaveBeenCalledWith("task-1", {
        workflow_id: "workflow-1",
        workflow_step_id: "review",
        position: 0,
      }),
    );
  });
});
