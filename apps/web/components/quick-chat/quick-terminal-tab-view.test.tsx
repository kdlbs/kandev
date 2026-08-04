import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const startHostShell = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    session_id: "session-1",
    agent_id: "_host_shell",
    cmd: ["/bin/bash"],
    running: true,
    started_at: "2026-08-04T00:00:00Z",
  }),
);

vi.mock("@/lib/api", () => ({ startHostShell }));
vi.mock("@/components/settings/pty-terminal-view", () => ({
  PtyTerminalView: (props: {
    startSession: (
      size: { cols: number; rows: number },
      options?: { clientId?: string },
    ) => Promise<unknown>;
    clientId?: string;
    lifecycle: string;
  }) => (
    <button
      type="button"
      data-testid="pty-view-probe"
      data-lifecycle={props.lifecycle}
      onClick={() => props.startSession({ cols: 80, rows: 24 }, { clientId: props.clientId })}
    >
      start
    </button>
  ),
}));

import { QuickTerminalTabView } from "./quick-terminal-tab-view";

const tab = {
  tabId: "6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b",
  workspaceId: "workspace-1",
  sessionId: null,
  sequence: 1,
  status: "connecting" as const,
};

describe("QuickTerminalTabView", () => {
  it("starts a host shell with the terminal tab's stable client id and detaches on unmount", () => {
    const onStateChange = vi.fn();
    const view = render(<QuickTerminalTabView tab={tab} onStateChange={onStateChange} />);

    expect(screen.getByTestId("pty-view-probe").getAttribute("data-lifecycle")).toBe(
      "detach-on-unmount",
    );
    fireEvent.click(screen.getByTestId("pty-view-probe"));
    expect(startHostShell).toHaveBeenCalledWith({ cols: 80, rows: 24 }, { clientId: tab.tabId });
    view.unmount();
  });
});
