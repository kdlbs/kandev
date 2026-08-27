import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { moveTask } from "@/lib/api";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

const { moveTaskMock } = vi.hoisted(() => ({ moveTaskMock: vi.fn() }));
const mocks = vi.hoisted(() => ({ touchDrawer: false }));

function Passthrough({ children }: { children: ReactNode }) {
  return <>{children}</>;
}

vi.mock("@/lib/api", () => ({
  moveTask: moveTaskMock,
}));

vi.mock("@kandev/ui/hover-card", () => ({
  HoverCard: Passthrough,
  HoverCardTrigger: Passthrough,
  HoverCardContent: Passthrough,
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
  mocks.touchDrawer = false;
});

// useToolbarCollapsed is mocked because the test DOM can't measure offsetWidth.
const collapsedMock = vi.fn(() => false);
vi.mock("@/hooks/use-toolbar-collapsed", () => ({
  useToolbarCollapsed: () => collapsedMock(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => undefined,
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
const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const DISCLOSURE_TEST_ID = "workflow-step-disclosure";
const MOVE_A_TEST_ID = "workflow-step-disclosure-move-a";
const MOVE_C_TEST_ID = "workflow-step-disclosure-move-c";
const MOVE_D_TEST_ID = "workflow-step-disclosure-move-d";
const TRIGGER_LABEL = "Step 2 of 4: Work";

describe("WorkflowStepper", () => {
  it("renders every step when there is room (not collapsed)", () => {
    collapsedMock.mockReturnValue(false);
    render(<WorkflowStepper steps={STEPS} currentStepId="b" />);

    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.queryByTestId("workflow-stepper-minimal")).toBeNull();
    // All steps render under the persistent outer container.
    expect(screen.getByTestId("workflow-step-Spec")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-Work")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-Review")).toBeTruthy();
  });

  it("collapses to only the current step when space runs out", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId="b" />);

    // Outer container persists across variants (stable e2e locator); minimal child marks collapsed state.
    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.getByTestId("workflow-stepper-minimal")).toBeTruthy();

    // Current step keeps its test id + aria-current in either variant.
    const current = screen.getByTestId("workflow-step-Work");
    expect(current.getAttribute("aria-current")).toBe("step");
    expect(screen.queryByTestId("workflow-step-Spec")).toBeNull();
    expect(screen.queryByTestId("workflow-step-Review")).toBeNull();

    // Position indicator reflects the current step out of the total.
    expect(screen.getByText("2/3")).toBeTruthy();
  });
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
    expect(screen.getByTestId("workflow-step-disclosure-row-b").getAttribute("aria-current")).toBe(
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

describe("WorkflowStepper fallback states", () => {
  it("falls back to the first step when collapsed with no current step", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={null} />);

    // Fallback step isn't the real current step, so it must not claim aria-current.
    expect(screen.getByTestId("workflow-step-Spec").getAttribute("aria-current")).toBeNull();
    expect(screen.getByText("1/3")).toBeTruthy();
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
    expect(screen.queryByTestId("workflow-step-Work")).toBeNull();
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
    moveTaskMock.mockResolvedValueOnce(undefined);
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
    moveTaskMock.mockResolvedValueOnce(undefined);
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
