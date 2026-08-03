import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickTerminalProvider } from "./quick-terminal-provider";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";

vi.mock("@/components/settings/host-shell-dialog", () => ({
  HostShellDialog: ({
    open,
    onOpenChange,
    presentation,
  }: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    presentation?: string;
  }) =>
    open ? (
      <div data-testid="quick-terminal-dialog" data-variant={presentation}>
        <button type="button" onClick={() => onOpenChange(false)}>
          close
        </button>
      </div>
    ) : null,
}));

function Launcher() {
  const openQuickTerminal = useQuickTerminalLauncher();
  return (
    <button type="button" onClick={openQuickTerminal}>
      launch
    </button>
  );
}

afterEach(() => cleanup());

describe("QuickTerminalProvider", () => {
  it("opens one lazy host-shell dialog in the quick presentation and closes it", async () => {
    render(
      <QuickTerminalProvider>
        <Launcher />
      </QuickTerminalProvider>,
    );

    expect(screen.queryByTestId("quick-terminal-dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "launch" }));

    const dialog = await screen.findByTestId("quick-terminal-dialog");
    expect(dialog.getAttribute("data-variant")).toBe("quick");

    fireEvent.click(screen.getByRole("button", { name: "close" }));
    expect(screen.queryByTestId("quick-terminal-dialog")).toBeNull();
  });
});
