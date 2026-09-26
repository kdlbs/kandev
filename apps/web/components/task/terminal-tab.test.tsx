import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { i18n } from "@/lib/i18n";

const mockDestroyUserShell = vi.fn();
const mockRenameUserShell = vi.fn();
const mockMarkTerminalPanelTerminateClose = vi.fn();
const mockClose = vi.fn();
const mockSetTitle = vi.fn();
const mockRemoveUserShell = vi.fn();
const mockToastError = vi.fn();
const CLOSE_TERMINAL_BUTTON_NAME = "Close Terminal";
const CONFIRM_CLOSE_BUTTON_NAME = "Close terminal";

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

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mockToastError(...args) },
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
  }) => (
    <div tabIndex={-1} {...props}>
      {children}
    </div>
  ),
  ContextMenuContent: ({
    children,
    onCloseAutoFocus,
  }: {
    children: React.ReactNode;
    onCloseAutoFocus?: (event: { preventDefault: () => void }) => void;
  }) => (
    <div
      onClick={() => {
        const preventDefault = vi.fn();
        onCloseAutoFocus?.({ preventDefault });
        if (!preventDefault.mock.calls.length) {
          document.querySelector<HTMLElement>('[data-testid="terminal-tab-shell-1"]')?.focus();
        }
      }}
    >
      {children}
    </div>
  ),
  ContextMenuItem: ({
    children,
    onClick,
    className,
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    className?: string;
  }) => (
    <button type="button" onClick={onClick} className={className}>
      {children}
    </button>
  ),
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

  afterEach(async () => {
    cleanup();
    await i18n.changeLanguage("en");
  });

  it("uses the localized ordinary-terminal title", async () => {
    await i18n.changeLanguage("pseudo");

    render(<TerminalTab {...makeProps()} />);

    await waitFor(() => {
      expect(mockSetTitle).toHaveBeenCalledWith(i18n.t("task:panelTerminal"));
    });
  });

  it("asks for localized confirmation before closing an idle terminal", async () => {
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));

    expect(screen.getByRole("dialog", { name: "Close terminal?" })).toBeTruthy();
    expect(mockDestroyUserShell).not.toHaveBeenCalled();
    expect(mockClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: CONFIRM_CLOSE_BUTTON_NAME }));

    await waitFor(() => {
      expect(mockDestroyUserShell).toHaveBeenCalledWith("env-1", "shell-1", "task-1");
      expect(mockClose).toHaveBeenCalledTimes(1);
    });
    expect(screen.queryByRole("status", { name: "Closing terminal" })).toBeNull();
    expect(screen.getByTestId("terminal-tab-shell-1").getAttribute("aria-busy")).toBeNull();
  });

  it("routes context-menu terminate through the same localized confirmation", async () => {
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Terminate" }));

    expect(screen.getByRole("dialog", { name: "Close terminal?" })).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(mockDestroyUserShell).not.toHaveBeenCalled();
    expect(mockClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: CONFIRM_CLOSE_BUTTON_NAME }));

    expect(screen.queryByRole("dialog", { name: "Close terminal?" })).toBeNull();
    expect(screen.queryByRole("status", { name: "Closing terminal" })).toBeNull();
    await waitFor(() => {
      expect(mockClose).toHaveBeenCalledTimes(1);
      expect(mockDestroyUserShell).toHaveBeenCalledTimes(1);
    });
  });

  it("focuses the inline rename input after the context menu closes", async () => {
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: /^Rename/ }));

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByTestId("terminal-tab-rename-input"));
    });
  });

  it("keeps the panel dismissed and reports an error if destroy fails", async () => {
    mockDestroyUserShell.mockRejectedValueOnce(new Error("network down"));
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));
    fireEvent.click(screen.getByRole("button", { name: CONFIRM_CLOSE_BUTTON_NAME }));

    await waitFor(() => expect(mockClose).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("status", { name: "Closing terminal" })).toBeNull();

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledTimes(1);
    });
    expect(mockToastError).toHaveBeenCalledWith("Failed to close terminal");
  });

  it("ignores a second confirmed terminate while close is in progress", async () => {
    render(<TerminalTab {...makeProps()} />);

    fireEvent.click(screen.getByRole("button", { name: CLOSE_TERMINAL_BUTTON_NAME }));
    fireEvent.click(screen.getByRole("button", { name: CONFIRM_CLOSE_BUTTON_NAME }));

    await waitFor(() => expect(mockDestroyUserShell).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: "Terminate" }));
    fireEvent.click(screen.getByRole("button", { name: CONFIRM_CLOSE_BUTTON_NAME }));

    await new Promise<void>((resolve) => queueMicrotask(resolve));
    expect(mockDestroyUserShell).toHaveBeenCalledTimes(1);
    expect(mockDestroyUserShell).toHaveBeenCalledWith("env-1", "shell-1", "task-1");
  });
});
