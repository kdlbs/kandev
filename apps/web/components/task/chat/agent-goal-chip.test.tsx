import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { AgentGoal } from "@/lib/agent-goal";
import { AgentGoalChip } from "./agent-goal-chip";

const responsiveMock = vi.hoisted(() => ({
  isMobile: false,
  isFinePointer: true,
}));
const GOAL_CHIP_TEST_ID = "agent-goal-chip";
const GOAL_POPOVER_TEST_ID = "agent-goal-popover";
const GOAL_OBJECTIVE = "Coordinate contributor PR reviews";
const GOAL_CONTINUATION = "The agent may continue automatically between replies.";

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => !responsiveMock.isFinePointer,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveMock.isMobile ? "mobile" : "desktop",
    isMobile: responsiveMock.isMobile,
    isTablet: false,
    isDesktop: !responsiveMock.isMobile,
    isCompactDesktop: false,
    isFullDesktop: !responsiveMock.isMobile,
    isFinePointer: responsiveMock.isFinePointer,
    usesDesktopWorkbench: !responsiveMock.isMobile,
  }),
}));

function activeGoal(overrides: Partial<AgentGoal> = {}): AgentGoal {
  return {
    objective: GOAL_OBJECTIVE,
    status: "active",
    createdAt: 10,
    updatedAt: 20,
    ...overrides,
  };
}

beforeEach(() => {
  responsiveMock.isMobile = false;
  responsiveMock.isFinePointer = true;
});

afterEach(() => {
  cleanup();
});

describe("AgentGoalChip", () => {
  it("opens the goal details on desktop hover", async () => {
    render(<AgentGoalChip goal={activeGoal()} />);

    const trigger = screen.getByTestId(GOAL_CHIP_TEST_ID);
    expect(trigger.className).toContain("h-6");

    fireEvent.mouseEnter(trigger);

    const details = await screen.findByTestId(GOAL_POPOVER_TEST_ID);
    expect(details.textContent).toContain(GOAL_OBJECTIVE);
    expect(details.textContent).toContain(GOAL_CONTINUATION);
  });

  it("uses a phone drawer with a large touch target", async () => {
    responsiveMock.isMobile = true;
    responsiveMock.isFinePointer = true;
    render(<AgentGoalChip goal={activeGoal()} />);

    const trigger = screen.getByTestId(GOAL_CHIP_TEST_ID);
    expect(trigger.className).toContain("min-h-11");
    expect(trigger.className).toContain("min-w-11");

    fireEvent.click(trigger);

    const details = await screen.findByTestId("agent-goal-drawer-content");
    expect(details.textContent).toContain(GOAL_OBJECTIVE);
    expect(screen.getByRole("button", { name: "Close goal details" }).className).toContain(
      "[@media(pointer:coarse)]:min-h-11",
    );
  });

  it("keeps hover details open while crossing into the popover", () => {
    vi.useFakeTimers();
    try {
      render(<AgentGoalChip goal={activeGoal()} />);

      const trigger = screen.getByTestId(GOAL_CHIP_TEST_ID);
      fireEvent.mouseEnter(trigger);
      const details = screen.getByTestId(GOAL_POPOVER_TEST_ID);

      fireEvent.mouseLeave(trigger);
      fireEvent.mouseEnter(details);
      act(() => vi.advanceTimersByTime(150));
      expect(screen.getByTestId(GOAL_POPOVER_TEST_ID)).toBe(details);

      fireEvent.mouseLeave(details);
      act(() => vi.advanceTimersByTime(150));
      expect(screen.queryByTestId(GOAL_POPOVER_TEST_ID)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("uses the drawer for coarse pointers and hides non-active goals", () => {
    responsiveMock.isFinePointer = false;
    const { rerender } = render(<AgentGoalChip goal={activeGoal()} />);

    expect(screen.getByTestId(GOAL_CHIP_TEST_ID).className).toContain("min-h-11");

    rerender(<AgentGoalChip goal={activeGoal({ status: "paused" })} />);
    expect(screen.queryByTestId(GOAL_CHIP_TEST_ID)).toBeNull();
  });
});
