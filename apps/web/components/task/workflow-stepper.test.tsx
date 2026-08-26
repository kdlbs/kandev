import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { moveTask } from "@/lib/api";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

const mocks = vi.hoisted(() => ({ touchDrawer: false }));

vi.mock("@/lib/api", () => ({
  moveTask: vi.fn(),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touchDrawer,
}));

vi.mock("@kandev/ui/hover-card", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const HoverCardContext = React.createContext<{
    open: boolean;
    onOpenChange: (open: boolean) => void;
  } | null>(null);

  return {
    HoverCard: ({
      children,
      open,
      onOpenChange,
    }: {
      children: React.ReactNode;
      open?: boolean;
      onOpenChange?: (open: boolean) => void;
    }) => {
      const [uncontrolledOpen, setUncontrolledOpen] = React.useState(false);
      const isOpen = open ?? uncontrolledOpen;
      const setOpen = onOpenChange ?? setUncontrolledOpen;
      return (
        <HoverCardContext.Provider value={{ open: isOpen, onOpenChange: setOpen }}>
          {children}
        </HoverCardContext.Provider>
      );
    },
    HoverCardTrigger: ({ children }: { children: React.ReactElement }) => {
      const context = React.useContext(HoverCardContext);
      return React.cloneElement(children as React.ReactElement<Record<string, unknown>>, {
        onMouseEnter: () => context?.onOpenChange(true),
        onFocus: () => context?.onOpenChange(true),
        onMouseLeave: () => context?.onOpenChange(false),
      });
    },
    HoverCardContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const context = React.useContext(HoverCardContext);
      return context?.open ? <div {...props}>{children}</div> : null;
    },
  };
});

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
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Step 2 of 4: Work" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.mouseEnter(trigger);

    expect(screen.getByTestId("workflow-step-disclosure")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-a")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-b").getAttribute("aria-current")).toBe(
      "step",
    );
    expect(screen.getByTestId("workflow-step-disclosure-row-c")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-row-d")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-move-a")).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-disclosure-move-b")).toBeNull();
    expect(screen.getByTestId("workflow-step-disclosure-move-c")).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-disclosure-move-d")).toBeNull();
  });

  it("moves an eligible target with the existing payload and closes after success", async () => {
    collapsedMock.mockReturnValue(true);
    vi.mocked(moveTask).mockResolvedValue({} as Awaited<ReturnType<typeof moveTask>>);
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );

    fireEvent.mouseEnter(screen.getByRole("button", { name: "Step 2 of 4: Work" }));
    fireEvent.click(screen.getByTestId("workflow-step-disclosure-move-c"));

    await waitFor(() =>
      expect(moveTask).toHaveBeenCalledWith("task-1", {
        workflow_id: "workflow-1",
        workflow_step_id: "c",
        position: 0,
      }),
    );
    await waitFor(() => expect(screen.queryByTestId("workflow-step-disclosure")).toBeNull());
  });

  it("uses the same step choices in a coarse-pointer drawer", () => {
    collapsedMock.mockReturnValue(true);
    mocks.touchDrawer = true;
    render(
      <WorkflowStepper
        steps={DISCLOSURE_STEPS}
        currentStepId="b"
        taskId="task-1"
        workflowId="workflow-1"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Step 2 of 4: Work" }));

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-move-a")).toBeTruthy();
    expect(screen.getByTestId("workflow-step-disclosure-move-c")).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-disclosure-move-d")).toBeNull();
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
        taskId="task-1"
        workflowId="workflow-1"
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
});
