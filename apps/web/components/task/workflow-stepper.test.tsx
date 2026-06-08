import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowStepper, type WorkflowStepperStep } from "./workflow-stepper";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// Drive the collapse decision directly instead of relying on layout
// measurement, which the test DOM can't perform (offsetWidth is always 0).
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

    // Outer container persists across variants (keeps the e2e locator stable);
    // the minimal child marks the collapsed state.
    expect(screen.getByTestId("workflow-stepper")).toBeTruthy();
    expect(screen.getByTestId("workflow-stepper-minimal")).toBeTruthy();

    // Only the current step is shown, and it keeps its test id + aria-current
    // so it stays selectable in either variant.
    const current = screen.getByTestId("workflow-step-Work");
    expect(current.getAttribute("aria-current")).toBe("step");
    expect(screen.queryByTestId("workflow-step-Spec")).toBeNull();
    expect(screen.queryByTestId("workflow-step-Review")).toBeNull();

    // Position indicator reflects the current step out of the total.
    expect(screen.getByText("2/3")).toBeTruthy();
  });

  it("falls back to the first step when collapsed with no current step", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId={null} />);

    expect(screen.getByTestId("workflow-step-Spec")).toBeTruthy();
    expect(screen.getByText("1/3")).toBeTruthy();
  });

  it("shows the archived badge instead of a step when collapsed and archived", () => {
    collapsedMock.mockReturnValue(true);
    render(<WorkflowStepper steps={STEPS} currentStepId="b" isArchived />);

    expect(screen.getByText("Archived")).toBeTruthy();
    expect(screen.queryByTestId("workflow-step-Work")).toBeNull();
  });

  it("renders nothing when there are no steps", () => {
    collapsedMock.mockReturnValue(false);
    const { container } = render(<WorkflowStepper steps={[]} currentStepId={null} />);
    expect(container.innerHTML).toBe("");
  });
});
