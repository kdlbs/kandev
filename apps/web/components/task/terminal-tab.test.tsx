import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const mockDestroyUserShell = vi.fn();
const mockRenameUserShell = vi.fn();
const mockMarkTerminalPanelTerminateClose = vi.fn();
const mockClose = vi.fn();
const mockSetTitle = vi.fn();
const mockRemoveUserShell = vi.fn();

const storeState = {
  tasks: { activeTaskId: "task-1" },
  userShells: {
    byEnvironmentId: {
      "env-1": [
        {
          terminalId: "shell-1",
          kind: "ordinary",
          seq: 1,
          customName: null,
          closable: true,
        },
      ],
    },
  },
  removeUserShell: mockRemoveUserShell,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  destroyUserShell: (...args: unknown[]) => mockDestroyUserShell(...args),
  renameUserShell: (...args: unknown[]) => mockRenameUserShell(...args),
}));

vi.mock("@/lib/terminal/terminal-busy-registry", () => ({
  shouldConfirmTerminalClose: () => false,
}));

vi.mock("./dockview-layout-setup", () => ({
  markTerminalPanelTerminateClose: (...args: unknown[]) =>
    mockMarkTerminalPanelTerminateClose(...args),
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: ({
    closeActionOverride,
    hideClose,
  }: {
    closeActionOverride?: () => void;
    hideClose?: boolean;
  }) =>
    hideClose ? null : (
      <button type="button" className="dv-default-tab-action" onClick={closeActionOverride}>
        close
      </button>
    ),
}));

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({
    children,
    ...props
  }: {
    children: React.ReactNode;
    [key: string]: unknown;
  }) => <div {...props}>{children}</div>,
  ContextMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuItem: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuSeparator: () => null,
}));

import { TerminalTab } from "./terminal-tab";

function makeProps() {
  return {
    api: {
      id: "panel-1",
      title: "Terminal",
      setTitle: mockSetTitle,
      close: mockClose,
    },
    params: {
      terminalId: "shell-1",
      taskID: "task-1",
      environmentId: "env-1",
    },
  } as unknown as React.ComponentProps<typeof TerminalTab>;
}

describe("TerminalTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDestroyUserShell.mockImplementation(() => new Promise<void>(() => {}));
  });

  afterEach(() => {
    cleanup();
  });

  it("replaces the close affordance with a spinner while destroy is pending", () => {
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "close" }));

    expect(mockDestroyUserShell).toHaveBeenCalledWith("env-1", "shell-1", "task-1");
    expect(screen.getByTestId("terminal-tab-closing-shell-1")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "close" })).toBeNull();
    expect(mockClose).not.toHaveBeenCalled();
  });
});
