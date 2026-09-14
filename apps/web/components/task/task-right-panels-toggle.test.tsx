import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  state: {
    isSupported: true,
    isReady: true,
    isMaximized: false,
    isAvailable: true,
    rightPanelsVisible: true,
    toggleRightPanels: vi.fn(),
  },
}));
const HIDE_RIGHT_PANE = "Hide right pane";
const SHOW_RIGHT_PANE = "Show right pane";
const RIGHT_PANE_UNAVAILABLE = "No separate right pane to hide";
const RIGHT_PANE_UNAVAILABLE_WHILE_MAXIMIZED =
  "Right pane is unavailable while a panel is maximized";
const TOGGLE_TEST_ID = "task-right-panels-toggle";
const ARIA_LABEL_ATTRIBUTE = "aria-label";

vi.mock("@/hooks/use-task-right-panels-toggle", () => ({
  useTaskRightPanelsToggle: () => mocks.state,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => {
      if (key === "task:hideRightPane") return HIDE_RIGHT_PANE;
      if (key === "task:showRightPane") return SHOW_RIGHT_PANE;
      if (key === "task:rightPaneUnavailable") return RIGHT_PANE_UNAVAILABLE;
      if (key === "task:rightPaneUnavailableWhileMaximized") {
        return RIGHT_PANE_UNAVAILABLE_WHILE_MAXIMIZED;
      }
      return key;
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
  mocks.state = {
    isSupported: true,
    isReady: true,
    isMaximized: false,
    isAvailable: true,
    rightPanelsVisible: true,
    toggleRightPanels: vi.fn(),
  };
});

describe("TaskRightPanelsToggle", () => {
  it("keeps the action in one button position and exposes the next action", () => {
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID);
    expect(button.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(HIDE_RIGHT_PANE);
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(button.getAttribute("title")).toBe(HIDE_RIGHT_PANE);

    button.focus();
    fireEvent.click(button);
    expect(mocks.state.toggleRightPanels).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(button);
  });

  it("switches to the show action when right panels are hidden", () => {
    mocks.state.rightPanelsVisible = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID);
    expect(button.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(SHOW_RIGHT_PANE);
    expect(button.getAttribute("aria-expanded")).toBe("false");
  });

  it("uses the shared touch-sized icon button", () => {
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
    expect(button.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBeNull();
    expect(button.getAttribute("title")).toBe(RIGHT_PANE_UNAVAILABLE_WHILE_MAXIMIZED);
    expect(button.parentElement?.tagName).toBe("SPAN");
    expect(button.parentElement?.getAttribute("tabindex")).toBe("0");
    expect(button.parentElement?.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(
      RIGHT_PANE_UNAVAILABLE_WHILE_MAXIMIZED,
    );
  });

  it("explains when the current layout has no separate right pane", () => {
    mocks.state.isAvailable = false;
    render(<TaskRightPanelsToggle sessionId="session-1" />);

    const button = screen.getByTestId(TOGGLE_TEST_ID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(button.getAttribute("title")).toBe(RIGHT_PANE_UNAVAILABLE);
    expect(button.parentElement?.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(RIGHT_PANE_UNAVAILABLE);
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
