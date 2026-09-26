import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { ParkedTerminalsMenu } from "./parked-terminals-menu";

const terminal: Terminal = {
  id: "shell-3",
  type: "shell",
  label: "Terminal 3",
  closable: true,
  kind: "ordinary",
  seq: 3,
  ptyStatus: "running",
};

describe("ParkedTerminalsMenu", () => {
  afterEach(cleanup);

  it("confirms a right-click destroy while preserving cancel", async () => {
    const onResume = vi.fn();
    const onDestroy = vi.fn();

    render(
      <ParkedTerminalsMenu
        parkedTerminals={[terminal]}
        onResume={onResume}
        onDestroy={onDestroy}
      />,
    );

    fireEvent.click(screen.getByTestId("parked-terminals-button"));
    fireEvent.contextMenu(screen.getByTestId(`parked-terminal-item-${terminal.id}`));

    const confirmation = screen.getByRole("group", { name: "Close terminal?" });
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("group", { name: "Close terminal?" })).toBeNull();
    expect(screen.getByTestId(`parked-terminal-item-${terminal.id}`).tagName).toBe("BUTTON");
    expect(onResume).not.toHaveBeenCalled();
    expect(onDestroy).not.toHaveBeenCalled();

    fireEvent.contextMenu(screen.getByTestId(`parked-terminal-item-${terminal.id}`));
    fireEvent.click(
      within(screen.getByRole("group", { name: "Close terminal?" })).getByRole("button", {
        name: "Close terminal",
      }),
    );

    await waitFor(() => expect(onDestroy).toHaveBeenCalledOnce());
    expect(onDestroy).toHaveBeenCalledWith(terminal.id);
    expect(screen.queryByTestId("parked-terminals-menu")).toBeNull();
  });
});
