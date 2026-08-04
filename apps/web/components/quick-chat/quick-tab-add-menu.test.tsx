import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickTabAddMenu } from "./quick-tab-add-menu";

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuItem: ({
    children,
    onSelect,
    ...props
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    [key: string]: unknown;
  }) => (
    <button type="button" {...props} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { count?: number; error?: string }) => {
      const messages: Record<string, string> = {
        "sidebar:quickChatAdd": "Add or switch tab",
        "sidebar:quickChatAgents": "Agents",
        "sidebar:quickChatNewAgent": "New Agent",
        "sidebar:quickChatTerminals": "Terminals",
        "sidebar:quickChatNewTerminal": "New Terminal",
        "sidebar:quickChatTerminalTab": `Terminal ${values?.count ?? ""}`,
        "sidebar:quickChatTerminalError": `Terminal error: ${values?.error ?? ""}`,
      };
      return messages[key] ?? key;
    },
  }),
}));

afterEach(() => cleanup());

describe("QuickTabAddMenu", () => {
  it("groups create actions and existing tabs while keeping the active row selectable", async () => {
    const onActivateSession = vi.fn();
    const onActivateTerminal = vi.fn();
    const onNewTerminal = vi.fn();
    render(
      <QuickTabAddMenu
        sessions={[{ sessionId: "chat-1", workspaceId: "ws-1", kind: "chat", name: "Chat one" }]}
        terminalTabs={[
          {
            tabId: "terminal-1",
            workspaceId: "ws-1",
            sessionId: "session-1",
            sequence: 1,
            status: "running",
          },
        ]}
        activeKind="terminal"
        activeSessionId="chat-1"
        activeTerminalTabId="terminal-1"
        sessionLabel={(session) => session.name ?? session.sessionId}
        onNewAgent={vi.fn()}
        onActivateSession={onActivateSession}
        onNewTerminal={onNewTerminal}
        onActivateTerminal={onActivateTerminal}
      />,
    );

    fireEvent.click(screen.getByTestId("quick-chat-add-menu-trigger"));

    expect(await screen.findByText("Agents")).toBeTruthy();
    expect(screen.getByText("New Agent")).toBeTruthy();
    expect(screen.getByText("Terminals")).toBeTruthy();
    expect(screen.getByText("New Terminal")).toBeTruthy();
    const terminalRow = screen.getByTestId("quick-chat-menu-terminal-terminal-1");
    expect(terminalRow.getAttribute("aria-current")).toBe("page");
    expect(terminalRow.getAttribute("data-disabled")).toBeNull();

    fireEvent.click(screen.getByTestId("quick-chat-menu-session-chat-1"));
    expect(onActivateSession).toHaveBeenCalledWith("chat-1");
    fireEvent.click(screen.getByTestId("quick-chat-new-terminal"));
    expect(onNewTerminal).toHaveBeenCalledTimes(1);
    fireEvent.click(terminalRow);
    expect(onActivateTerminal).toHaveBeenCalledWith("terminal-1");
  });
});
