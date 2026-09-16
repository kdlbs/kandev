import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

const POPOVER_TEST_ID = "workflow-step-popover";
const REVIEW_TRIGGER_TEST_ID = "workflow-step-Review";
const EXPANDED_ATTRIBUTE = "aria-expanded";

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
        expect(screen.getByTestId(POPOVER_TEST_ID)).toBeTruthy();
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
    const destinationTrigger = screen.getByTestId(REVIEW_TRIGGER_TEST_ID);
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

    const destinationTrigger = screen.getByTestId(REVIEW_TRIGGER_TEST_ID);
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

describe("WorkflowStepper exclusive hover", () => {
  it("replaces a focused step hover when the pointer enters another step", async () => {
    vi.useFakeTimers();
    try {
      render(
        <WorkflowStepper
          steps={STEPS}
          currentStepId="work"
          taskId="task-1"
          workflowId="workflow-1"
        />,
      );
      const work = screen.getByTestId("workflow-step-Work");
      const review = screen.getByTestId(REVIEW_TRIGGER_TEST_ID);
      act(() => work.focus());
      await act(() => vi.advanceTimersByTimeAsync(200));
      expect(work.getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");

      fireEvent.pointerEnter(review);
      await act(() => vi.advanceTimersByTimeAsync(200));
      expect(review.getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
      expect(screen.getAllByTestId(POPOVER_TEST_ID)).toHaveLength(1);
      expect(work.getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");

      // The old close timer and focus restoration must not reclaim the disclosure.
      fireEvent.pointerLeave(review);
      fireEvent.pointerEnter(work);
      await act(() => vi.advanceTimersByTimeAsync(200));
      expect(work.getAttribute(EXPANDED_ATTRIBUTE)).toBe("true");
      expect(review.getAttribute(EXPANDED_ATTRIBUTE)).toBe("false");
      expect(screen.getAllByTestId(POPOVER_TEST_ID)).toHaveLength(1);
    } finally {
      cleanup();
      vi.useRealTimers();
    }
  });
});

describe("WorkflowStepper hover dismissal", () => {
  it.each([false, true])(
    "stays closed after pointer exit (entered content: %s)",
    async (enterContent) => {
      vi.useFakeTimers();
      try {
        render(
          <WorkflowStepper
            steps={STEPS}
            currentStepId="work"
            taskId="task-1"
            workflowId="workflow-1"
          />,
        );
        const trigger = screen.getByTestId("workflow-step-Work");
        fireEvent.pointerEnter(trigger);
        await act(() => vi.advanceTimersByTimeAsync(200));
        const popover = screen.getByTestId(POPOVER_TEST_ID);
        if (enterContent) fireEvent.pointerEnter(popover);
        fireEvent.pointerLeave(trigger);
        if (enterContent) fireEvent.pointerLeave(popover);
        await act(() => vi.advanceTimersByTimeAsync(100));
        expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull();
        await act(() => vi.advanceTimersByTimeAsync(500));
        expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull();
        expect(document.activeElement).not.toBe(trigger);
      } finally {
        cleanup();
        vi.useRealTimers();
      }
    },
  );

  it("returns keyboard focus on Escape without reopening", async () => {
    vi.useFakeTimers();
    try {
      render(
        <WorkflowStepper
          steps={STEPS}
          currentStepId="work"
          taskId="task-1"
          workflowId="workflow-1"
        />,
      );
      const trigger = screen.getByTestId(REVIEW_TRIGGER_TEST_ID);
      act(() => trigger.focus());
      await act(() => vi.advanceTimersByTimeAsync(200));
      const move = screen.getByTestId("workflow-step-move-here");
      act(() => move.focus());
      fireEvent.keyDown(move, { key: "Escape" });
      await act(() => vi.advanceTimersByTimeAsync(500));
      expect(screen.queryByTestId(POPOVER_TEST_ID)).toBeNull();
      expect(document.activeElement).toBe(trigger);
    } finally {
      cleanup();
      vi.useRealTimers();
    }
  });
});
