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
        "common:agents": "Agents",
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
  it("offers creation actions without duplicating the tab strip", async () => {
    const onNewTerminal = vi.fn();
    render(<QuickTabAddMenu onNewAgent={vi.fn()} onNewTerminal={onNewTerminal} />);

    fireEvent.click(screen.getByTestId("quick-chat-add-menu-trigger"));

    expect(await screen.findByText("Agents")).toBeTruthy();
    expect(screen.getByText("New Agent")).toBeTruthy();
    expect(screen.getByText("Terminals")).toBeTruthy();
    expect(screen.getByText("New Terminal")).toBeTruthy();
    fireEvent.click(screen.getByTestId("quick-chat-new-terminal"));
    expect(onNewTerminal).toHaveBeenCalledTimes(1);
  });
});
