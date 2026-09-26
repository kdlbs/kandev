import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickTabAddMenu } from "./quick-tab-add-menu";

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children, align }: { children: React.ReactNode; align?: string }) => (
    <div data-testid="quick-chat-add-menu-content" data-align={align}>
      {children}
    </div>
  ),
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

afterEach(() => cleanup());

describe("QuickTabAddMenu", () => {
  it("offers creation actions without duplicating the tab strip", async () => {
    const onNewTerminal = vi.fn();
    render(<QuickTabAddMenu onNewAgent={vi.fn()} onNewTerminal={onNewTerminal} />);

    fireEvent.click(screen.getByTestId("quick-chat-add-menu-trigger"));

    expect(await screen.findByText("Agents")).toBeTruthy();
    expect(screen.getByTestId("quick-chat-add-menu-content").getAttribute("data-align")).toBe(
      "start",
    );
    expect(screen.getByText("New Agent")).toBeTruthy();
    expect(screen.getByText("Terminals")).toBeTruthy();
    expect(screen.getByText("New Terminal")).toBeTruthy();
    fireEvent.click(screen.getByTestId("quick-chat-new-terminal"));
    expect(onNewTerminal).toHaveBeenCalledTimes(1);
  });
});
