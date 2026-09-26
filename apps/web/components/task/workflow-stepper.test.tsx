import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { moveTask } from "@/lib/api";
import type { WorkflowMovePreviewResponse } from "@/lib/api";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

/* eslint-disable max-lines -- shared desktop and touch stepper coverage uses one fixture. */

const { moveTaskMock, previewWorkflowMoveMock, appStoreState } = vi.hoisted(() => ({
  moveTaskMock: vi.fn(),
  previewWorkflowMoveMock: vi.fn().mockResolvedValue(undefined),
  appStoreState: {
    connection: { status: "connected", error: null, issueSeverity: "none" },
    workspaceContextGeneration: 1,
    workflows: { items: [], activeId: null },
    tasks: { activeSessionId: null },
    taskRemoval: { navigationRevision: 0 },
    beginWorkflowSessionFocus: vi.fn(() => 1),
    bindWorkflowSessionFocus: vi.fn(),
    reconcileWorkflowSessionFocus: vi.fn(),
    cancelWorkflowSessionFocus: vi.fn(),
    chatInput: { planModeBySessionId: {} },
    kanban: { tasks: [] },
    kanbanMulti: { snapshots: {} },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {} },
    agentProfiles: { items: [] },
    setPlanMode: vi.fn(),
    setActiveDocument: vi.fn(),
  },
}));
const mocks = vi.hoisted(() => ({ touchDrawer: false }));

function Passthrough({ children }: { children: ReactNode }) {
  return <>{children}</>;
}

vi.mock("@/lib/api", () => ({
  moveTask: moveTaskMock,
  previewWorkflowMove: previewWorkflowMoveMock,
}));

vi.mock("@kandev/ui/hover-card", () => ({
  HoverCard: Passthrough,
  HoverCardTrigger: Passthrough,
  HoverCardContent: Passthrough,
}));

vi.mock("@/components/integrations/use-hover-popover", () => ({
  useHoverPopover: () => ({
    open: true,
    onOpenChange: vi.fn(),
    onTriggerEnter: vi.fn(),
    onTriggerLeave: vi.fn(),
    onContentEnter: vi.fn(),
    onContentLeave: vi.fn(),
  }),
}));

// The stepper only threads the shared move-options draft through the hover
// popover; the fields themselves are covered by the form's own tests.
vi.mock("./workflow-move-options", () => ({
  useWorkflowMoveOptionsForm: () => ({
    draft: {},
    patchDraft: vi.fn(),
    resetDraft: vi.fn(),
  }),
  WorkflowMoveOptionsFields: () => null,
  workflowMoveOptionsPayload: () => undefined,
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touchDrawer,
}));

vi.mock("@kandev/ui/drawer", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const DrawerContext = React.createContext<{
    open: boolean;
    onOpenChange: (open: boolean) => void;
  } | null>(null);

  return {
    Drawer: ({
      children,
      open,
      onOpenChange,
    }: {
      children: React.ReactNode;
      open: boolean;
      onOpenChange: (open: boolean) => void;
    }) => (
      <DrawerContext.Provider value={{ open, onOpenChange }}>{children}</DrawerContext.Provider>
    ),
    DrawerTrigger: ({ children }: { children: React.ReactElement }) => {
      const context = React.useContext(DrawerContext);
      return React.cloneElement(children as React.ReactElement<Record<string, unknown>>, {
        onClick: () => context?.onOpenChange(true),
      });
    },
    DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const context = React.useContext(DrawerContext);
      return context?.open ? (
        <div role="dialog" {...props}>
          {children}
        </div>
      ) : null;
    },
    DrawerHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
      <div {...props}>{children}</div>
    ),
    DrawerTitle: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
      <h2 {...props}>{children}</h2>
    ),
    DrawerDescription: ({ children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) => (
      <p {...props}>{children}</p>
    ),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  previewWorkflowMoveMock.mockResolvedValue(undefined);
  mocks.touchDrawer = false;
  appStoreState.kanban.tasks.length = 0;
  const taskSessions = appStoreState.taskSessions.items as Record<string, unknown>;
  for (const key of Object.keys(taskSessions)) {
    delete taskSessions[key];
  }
  appStoreState.agentProfiles.items.length = 0;
});

// useToolbarCollapsed is mocked because the test DOM can't measure offsetWidth.
const collapsedMock = vi.fn(() => false);
vi.mock("@/hooks/use-toolbar-collapsed", () => ({
  useToolbarCollapsed: () => collapsedMock(),
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

const STEPS: WorkflowStepperStep[] = [
  { id: "a", name: "Spec", color: "#111", position: 0 },
  { id: "b", name: "Work", color: "#222", position: 1 },
  { id: "c", name: "Review", color: "#333", position: 2 },
];

const DISCLOSURE_STEPS: WorkflowStepperStep[] = [
  ...STEPS,
  { id: "d", name: "Done", color: "#444", position: 3, allow_manual_move: false },
];
const LATER_MOVABLE_DISCLOSURE_STEPS: WorkflowStepperStep[] = [
  ...DISCLOSURE_STEPS,
  { id: "e", name: "Later", color: "#555", position: 4, allow_manual_move: true },
];
const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const DISCLOSURE_TEST_ID = "workflow-step-disclosure";
const MOVE_A_TEST_ID = "workflow-step-disclosure-move-a";
const MOVE_C_TEST_ID = "workflow-step-disclosure-move-c";
const MOVE_D_TEST_ID = "workflow-step-disclosure-move-d";
const TRIGGER_LABEL = /Step 2 of 4: Work/;
const SPEC_TEST_ID = "workflow-step-Spec";
const ARIA_CURRENT = "aria-current";
const WORK_TEST_ID = "workflow-step-Work";
const MARKER_FOOTPRINT_HEIGHT = "h-2";
const MARKER_FOOTPRINT_WIDTH = "w-2";
const SPINNER_HEIGHT = "h-3.5";
const SPINNER_WIDTH = "w-3.5";
const MARKER_SELECTOR = "[data-marker-state]";

function replacementModelPreview(): WorkflowMovePreviewResponse {
  return {
    task_id: TASK_ID,
    workflow_step_id: "c",
    evaluated_at: "2026-09-19T00:00:00Z",
    outcome: "create_new",
    model: {
      before: { known: false },
      after: { known: true, label: "gpt-5.6-terra" },
      after_source: "profile",
    },
    context_reset: false,
    context_reset_state: "unchanged",
    source_disposition: "park",
    dispatch: "prompt",
  };
}

describe("WorkflowStepper", () => {
  it("shows a pending marker while a move request is unresolved", async () => {
    collapsedMock.mockReturnValue(false);
    let resolveMove!: () => void;
    moveTaskMock.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveMove = resolve;
      }),
    );

    render(
      <WorkflowStepper steps={STEPS} currentStepId="b" taskId={TASK_ID} workflowId={WORKFLOW_ID} />,
    );

    fireEvent.click(screen.getAllByRole("button", { name: "Move here" })[0]);

    await waitFor(() =>
      expect(
        screen
          .getByTestId("workflow-step-Spec")
          .querySelector(MARKER_SELECTOR)
          ?.getAttribute("data-marker-state"),
      ).toBe("pending"),
    );
    const marker = screen.getByTestId("workflow-step-Spec").querySelector(MARKER_SELECTOR);
    expect(
      marker?.classList.contains(MARKER_FOOTPRINT_HEIGHT) &&
        marker?.classList.contains(MARKER_FOOTPRINT_WIDTH),
    ).toBe(true);
    const spinner = marker?.querySelector("svg");
    expect(spinner?.getAttribute("data-marker-visual-size")).toBe("8");
    expect(spinner?.classList.contains(MARKER_FOOTPRINT_HEIGHT)).toBe(true);
    expect(spinner?.classList.contains(MARKER_FOOTPRINT_WIDTH)).toBe(true);

    resolveMove();
  });

  it("keeps the current pending spinner's 14px outer decoration without changing its 8px footprint", () => {
    collapsedMock.mockReturnValue(false);
    (appStoreState.kanban.tasks as unknown[]).push({
      id: TASK_ID,
      state: "SCHEDULING",
      workflowStepId: "b",
    });

    render(
      <WorkflowStepper steps={STEPS} currentStepId="b" taskId={TASK_ID} workflowId={WORKFLOW_ID} />,
    );

    const marker = screen.getByTestId(WORK_TEST_ID).querySelector(MARKER_SELECTOR);
    expect(
      marker?.classList.contains(MARKER_FOOTPRINT_HEIGHT) &&
        marker?.classList.contains(MARKER_FOOTPRINT_WIDTH),
    ).toBe(true);
    const spinner = marker?.querySelector("svg");
    expect(spinner?.getAttribute("data-marker-visual-size")).toBe("14");
    expect(spinner?.classList.contains(SPINNER_HEIGHT)).toBe(true);
    expect(spinner?.classList.contains(SPINNER_WIDTH)).toBe(true);
  });

  it("renders every step when there is room (not collapsed)", () => {
    collapsedMock.mockReturnValue(false);
    render(<WorkflowStepper steps={STEPS} currentStepId="b" />);

    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.queryByTestId("workflow-stepper-minimal")).toBeNull();
    // All steps render under the persistent outer container.
    expect(screen.getByTestId(SPEC_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(WORK_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("workflow-step-Review")).toBeTruthy();
  });

  it("orders steps sharing a position by ascending id, not insertion order", () => {
    collapsedMock.mockReturnValue(false);
    const TIEBREAK_STEPS: WorkflowStepperStep[] = [
      { id: "z", name: "ZStep", color: "#111", position: 0 },
      { id: "a", name: "AStep", color: "#222", position: 0 },
    ];
    const { container } = render(<WorkflowStepper steps={TIEBREAK_STEPS} currentStepId={null} />);

    const aIndex = container.innerHTML.indexOf("AStep");
    const zIndex = container.innerHTML.indexOf("ZStep");
    expect(aIndex).toBeGreaterThan(-1);
    expect(aIndex).toBeLessThan(zIndex);
  });

  it("collapses to only the current step when space runs out", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId="b" />);

    // Outer container persists across variants (stable e2e locator); minimal child marks collapsed state.
    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.getByTestId("workflow-stepper-minimal")).toBeTruthy();

    // Current step keeps its test id + aria-current in either variant.
    const current = screen.getByTestId(WORK_TEST_ID);
    expect(current.getAttribute(ARIA_CURRENT)).toBe("step");
    expect(screen.queryByTestId(SPEC_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("workflow-step-Review")).toBeNull();

    // Position indicator reflects the current step out of the total.
    expect(screen.getByText("2/3")).toBeTruthy();
  });
});

describe("WorkflowStepper progress disclosure", () => {
  it("renders the planned replacement model in the desktop topbar disclosure", async () => {
    collapsedMock.mockReturnValue(false);
    previewWorkflowMoveMock.mockResolvedValue(replacementModelPreview());

    render(
      <WorkflowStepper steps={STEPS} currentStepId="b" taskId={TASK_ID} workflowId={WORKFLOW_ID} />,
    );

    fireEvent.mouseEnter(screen.getByTestId("workflow-step-Review"));

    await waitFor(() =>
      expect(
        screen
          .getAllByTestId("workflow-move-preview")
          .some((preview) => preview.textContent?.includes("gpt-5.6-terra")),
      ).toBe(true),
    );
  });

  it("renders the planned replacement model after a touch disclosure tap", async () => {
    collapsedMock.mockReturnValue(true);
    mocks.touchDrawer = true;
    previewWorkflowMoveMock.mockResolvedValue(replacementModelPreview());

    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: TRIGGER_LABEL }));

    await waitFor(() =>
      expect(
        screen
          .getAllByTestId("workflow-move-preview")
          .some((preview) => preview.textContent?.includes("gpt-5.6-terra")),
      ).toBe(true),
    );
  });

  it("keeps lifecycle details inside the existing disclosure", () => {
    collapsedMock.mockReturnValue(true);
    (appStoreState.kanban.tasks as unknown[]).push({
      id: TASK_ID,
      workflowId: WORKFLOW_ID,
      workflowStepId: "b",
      state: "SCHEDULING",
    });
    (appStoreState.agentProfiles.items as unknown[]).push({ id: "profile-1", label: "Luna" });
    const steps = DISCLOSURE_STEPS.map((step) =>
      step.id === "b" ? { ...step, agent_profile_id: "profile-1" } : step,
    );

    render(
      <WorkflowStepper steps={steps} currentStepId="b" taskId={TASK_ID} workflowId={WORKFLOW_ID} />,
    );

    expect(screen.queryByText("Preparing agent")).toBeNull();
    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));

    expect(screen.getByTestId("workflow-step-progress-b").textContent).toContain("Preparing agent");
    expect(screen.getByTestId("workflow-step-progress-b").textContent).toContain("Luna");
    expect(screen.getByTestId("workflow-step-disclosure-row-b").getAttribute("aria-current")).toBe(
      "step",
    );
  });

  it.each([
    ["STARTING", true, "Stopping agent"],
    ["FAILED", false, "Agent failed"],
  ] as const)(
    "renders %s lifecycle status in the existing full disclosure",
    (sessionState, cancellationPending, label) => {
      collapsedMock.mockReturnValue(false);
      (appStoreState.kanban.tasks as unknown[]).push({
        id: TASK_ID,
        state: "IN_PROGRESS",
        workflowStepId: "b",
        primarySessionId: "session-1",
      });
      (appStoreState.taskSessions.items as Record<string, unknown>)["session-1"] = {
        id: "session-1",
        task_id: TASK_ID,
        state: sessionState,
        is_primary: true,
        cancellation_pending: cancellationPending,
      };

      render(
        <WorkflowStepper
          steps={STEPS}
          currentStepId="b"
          taskId={TASK_ID}
          workflowId={WORKFLOW_ID}
        />,
      );

      expect(screen.getByTestId("workflow-step-progress-b").textContent).toContain(label);
    },
  );
});

describe("WorkflowStepper compact disclosure", () => {
  it("opens every ordered step for fine-pointer users and marks only eligible targets", () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    const trigger = screen.getByRole("button", { name: TRIGGER_LABEL });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.mouseEnter(trigger);

    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-a")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-b").getAttribute(ARIA_CURRENT)).toBe(
      "step",
    );
    expect(screen.getByTestId("workflow-step-disclosure-row-c")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-d")).toBeTruthy();
    expect(screen.getByTestId(MOVE_A_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-disclosure-move-b")).toBeNull();
    expect(screen.getByTestId(MOVE_C_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(MOVE_D_TEST_ID)).toBeNull();
  });

  it("moves an eligible target with the existing payload and closes after success", async () => {
    collapsedMock.mockReturnValue(true);
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));
    fireEvent.click(screen.getByTestId(MOVE_C_TEST_ID));

    await waitFor(() =>
      expect(moveTask).toHaveBeenCalledWith(TASK_ID, {
        workflow_id: WORKFLOW_ID,
        workflow_step_id: "c",
        position: 0,
      }),
    );
    await waitFor(() => expect(screen.queryByTestId(DISCLOSURE_TEST_ID)).toBeNull());
  });

  it("keeps the disclosure open and re-enables the target after a failed move", async () => {
    collapsedMock.mockReturnValue(true);
    vi.mocked(moveTask).mockRejectedValue(new Error("network error"));
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));
    const moveButton = screen.getByTestId(MOVE_C_TEST_ID);
    fireEvent.click(moveButton);

    await waitFor(() => expect(moveButton.hasAttribute("disabled")).toBe(false));
    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();
  });

  it("uses the same step choices in a coarse-pointer drawer", () => {
    collapsedMock.mockReturnValue(true);
    mocks.touchDrawer = true;
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: TRIGGER_LABEL }));

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(MOVE_A_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(MOVE_C_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(MOVE_D_TEST_ID)).toBeNull();
  });
});

describe("WorkflowStepper compact disclosure options", () => {
  it("reveals opt-in one-time move options per eligible target", () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));

    // Only eligible, non-current targets expose the options toggle.
    expect(screen.getByTestId("workflow-step-disclosure-options-c")).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-disclosure-options-b")).toBeNull();
    expect(screen.queryByTestId("workflow-step-disclosure-options-d")).toBeNull();

    // The options panel stays hidden until the toggle is pressed.
    expect(screen.queryByTestId("workflow-step-disclosure-options-panel-c")).toBeNull();
    fireEvent.click(screen.getByTestId("workflow-step-disclosure-options-c"));
    expect(screen.getByTestId("workflow-step-disclosure-options-panel-c")).toBeTruthy();
  });

  it("keeps the direct move action before the options toggle in the DOM", () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));

    const row = screen.getByTestId("workflow-step-disclosure-row-c");
    expect(
      Array.from(row.querySelectorAll<HTMLButtonElement>("button")).map(
        (button) => button.dataset.testid,
      ),
    ).toEqual(["workflow-step-disclosure-move-c", "workflow-step-disclosure-options-c"]);
  });

  it("keeps completed and future non-movable labels visually muted", () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: TRIGGER_LABEL }));

    expect(screen.getByText("Spec").className).toContain("text-muted-foreground");
    expect(screen.getByText("Done").className).toContain("text-muted-foreground/60");
  });
});

describe("WorkflowStepper compact disclosure preview queue", () => {
  it("requests a later movable row after the first two destinations", async () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={LATER_MOVABLE_DISCLOSURE_STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: /Step 2 of 5/ }));
    const disclosure = screen.getByTestId(DISCLOSURE_TEST_ID);
    expect(screen.getByTestId("workflow-step-disclosure-row-e")).toBeTruthy();
    fireEvent.scroll(disclosure);

    await waitFor(() => {
      expect(
        previewWorkflowMoveMock.mock.calls.some(([, payload]) => payload?.workflow_step_id === "e"),
      ).toBe(true);
    });
  });
});

describe("WorkflowStepper fallback states", () => {
  it("falls back to the first step when collapsed with no current step", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={null} />);

    // Fallback step isn't the real current step, so it must not claim aria-current.
    const fallbackStep = screen.getByTestId(SPEC_TEST_ID);
    expect(fallbackStep.getAttribute(ARIA_CURRENT)).toBeNull();
    // aria-current alone is a false negative here: it was already conditioned
    // correctly before the fix. The marker itself must also not read "current".
    expect(
      fallbackStep.querySelector("[data-marker-state]")?.getAttribute("data-marker-state"),
    ).toBe("upcoming");
    expect(screen.getByText("1/3")).toBeTruthy();
  });

  it("marks the marker current when collapsed on a resolved current step", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId="a" />);

    const currentStep = screen.getByTestId(SPEC_TEST_ID);
    expect(currentStep.getAttribute(ARIA_CURRENT)).toBe("step");
    expect(
      currentStep.querySelector("[data-marker-state]")?.getAttribute("data-marker-state"),
    ).toBe("current");
  });

  it("shows the archived badge instead of a step when collapsed and archived", () => {
    collapsedMock.mockReturnValue(true);
    render(
      <WorkflowStepper
        steps={STEPS}
        currentStepId="b"
        taskId={TASK_ID}
        workflowId={WORKFLOW_ID}
        isArchived
      />,
    );

    expect(screen.getByText("Archived")).toBeTruthy();
    // Archived badge carries the minimal test id for collapsed-mode detection.
    expect(screen.getByTestId("workflow-stepper-minimal")).toBeTruthy();
    expect(screen.queryByTestId(WORK_TEST_ID)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("renders nothing when there are no steps", () => {
    collapsedMock.mockReturnValue(false);
    const { container } = render(<WorkflowStepper steps={[]} currentStepId={null} />);
    expect(container.innerHTML).toBe("");
  });

  it("reports a rejected move to the owning surface", async () => {
    const error = new Error("task has an active session (RUNNING)");
    moveTaskMock.mockRejectedValueOnce(error);
    const onMoveError = vi.fn();
    const props = {
      steps: STEPS,
      currentStepId: "b",
      taskId: TASK_ID,
      workflowId: WORKFLOW_ID,
      onMoveError,
    } as ComponentProps<typeof WorkflowStepper> & { onMoveError: typeof onMoveError };

    render(<WorkflowStepper {...props} />);
    const moveButton = screen.getAllByRole("button", { name: "Move here" })[0];
    fireEvent.click(moveButton);

    await waitFor(() => expect(onMoveError).toHaveBeenCalledWith(error));
    expect(moveButton.hasAttribute("disabled")).toBe(false);
  });

  it("ignores a superseded move's rejection so it cannot overwrite a newer result", async () => {
    // Only the in-flight step's own button is disabled, so the user can start a
    // second move before the first resolves. A rejection from the abandoned
    // request must not paint a banner describing a move nobody is waiting on.
    let rejectFirst!: (error: unknown) => void;
    moveTaskMock.mockReturnValueOnce(new Promise((_res, rej) => (rejectFirst = rej)));
    moveTaskMock.mockResolvedValueOnce({});
    const onMoveError = vi.fn();
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const props = {
      steps: STEPS,
      currentStepId: "b",
      taskId: TASK_ID,
      workflowId: WORKFLOW_ID,
      onMoveError,
    } as ComponentProps<typeof WorkflowStepper> & { onMoveError: typeof onMoveError };

    render(<WorkflowStepper {...props} />);
    const moveButtons = screen.getAllByRole("button", { name: "Move here" });
    fireEvent.click(moveButtons[0]);
    fireEvent.click(moveButtons[1]);

    await waitFor(() => expect(moveTaskMock).toHaveBeenCalledTimes(2));
    rejectFirst(new Error("task has an active session (RUNNING)"));
    await waitFor(() => expect(consoleErrorSpy).toHaveBeenCalled());

    expect(onMoveError).not.toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });

  it("notifies the owning surface when a move starts", () => {
    const onMoveStart = vi.fn();
    moveTaskMock.mockResolvedValueOnce({});
    const props = {
      steps: STEPS,
      currentStepId: "b",
      taskId: TASK_ID,
      workflowId: WORKFLOW_ID,
      onMoveStart,
    } as ComponentProps<typeof WorkflowStepper> & { onMoveStart: typeof onMoveStart };

    render(<WorkflowStepper {...props} />);
    fireEvent.click(screen.getAllByRole("button", { name: "Move here" })[0]);

    expect(onMoveStart).toHaveBeenCalledTimes(1);
  });
});
