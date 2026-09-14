import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  isFinePointer: true,
  state: {
    isSupported: true,
    isReady: true,
    isMaximized: false,
    rightPanelsVisible: true,
    toggleRightPanels: vi.fn(),
  },
}));
const HIDE_RIGHT_PANELS = "Hide right panels";
const SHOW_RIGHT_PANELS = "Show right panels";
const RIGHT_PANELS_UNAVAILABLE = "Right panels are unavailable while a panel is maximized";
const TOGGLE_TEST_ID = "task-right-panels-toggle";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.isFinePointer }),
}));

vi.mock("@/hooks/use-task-right-panels-toggle", () => ({
  useTaskRightPanelsToggle: () => mocks.state,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => {
      if (key === "task:hideRightPanels") return HIDE_RIGHT_PANELS;
      if (key === "task:rightPanelsUnavailableWhileMaximized") return RIGHT_PANELS_UNAVAILABLE;
      return SHOW_RIGHT_PANELS;
    },
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { TaskRightPanelsToggle } from "./task-right-panels-toggle";

afterEach(cleanup);

beforeEach(() => {
  mocks.isFinePointer = true;
  mocks.state = {
    isSupported: true,
    isReady: true,
    isMaximized: false,
    rightPanelsVisible: true,
    toggleRightPanels: vi.fn(),
  };
});

describe("TaskRightPanelsToggle", () => {
  it("keeps the action in one button position and exposes the next action", () => {
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID);
    expect(button.getAttribute("aria-label")).toBe(HIDE_RIGHT_PANELS);
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(button.getAttribute("title")).toBe(HIDE_RIGHT_PANELS);

    button.focus();
    fireEvent.click(button);
    expect(mocks.state.toggleRightPanels).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(button);
  });

  it("switches to the show action when right panels are hidden", () => {
    mocks.state.rightPanelsVisible = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID);
    expect(button.getAttribute("aria-label")).toBe(SHOW_RIGHT_PANELS);
    expect(button.getAttribute("aria-expanded")).toBe("false");
  });

  it("keeps coarse-pointer controls at the touch target size", () => {
    mocks.isFinePointer = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    expect(screen.getByTestId(TOGGLE_TEST_ID).className).toContain(
      "[@media(pointer:coarse)]:size-11",
    );
  });

  it("disables the action while the selected layout is not ready", () => {
    mocks.state.isReady = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    expect((screen.getByTestId(TOGGLE_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
  });

  it("explains why the action is disabled while a panel is maximized", () => {
    mocks.state.isMaximized = true;
    mocks.state.isReady = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-label")).toBe(RIGHT_PANELS_UNAVAILABLE);
    expect(button.getAttribute("title")).toBe(RIGHT_PANELS_UNAVAILABLE);
  });

  it("restores focus after a layout transition disables the button", () => {
    const toggleRightPanels = vi.fn(() => {
      mocks.state.isReady = false;
    });
    mocks.state.toggleRightPanels = toggleRightPanels;
    const { rerender } = render(<TaskRightPanelsToggle sessionId="session-1" />);
    const button = screen.getByTestId(TOGGLE_TEST_ID);

    button.focus();
    fireEvent.click(button);
    rerender(<TaskRightPanelsToggle sessionId="session-1" />);
    expect((button as HTMLButtonElement).disabled).toBe(true);

    mocks.state.isReady = true;
    rerender(<TaskRightPanelsToggle sessionId="session-1" />);
    expect(document.activeElement).toBe(button);
  });

  it("does not render on phones", () => {
    mocks.state.isSupported = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    expect(screen.queryByTestId(TOGGLE_TEST_ID)).toBeNull();
  });
});
