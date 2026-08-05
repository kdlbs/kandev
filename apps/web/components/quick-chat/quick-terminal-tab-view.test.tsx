import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
const createQuickTerminalTab = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    tabId: "6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b",
    workspaceId: "workspace-1",
    sessionId: null,
    sequence: 1,
    status: "connecting",
  }),
);
const deleteQuickTerminalTab = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));

vi.mock("@/lib/api", () => ({
  startHostShell,
  createQuickTerminalTab,
  deleteQuickTerminalTab,
  toQuickTerminalTab: (tab: unknown) => tab,
}));
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
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { code?: number; error?: string }) => {
      if (key === "sidebar:quickChatTerminalConnecting") return "Connecting to terminal…";
      if (key === "sidebar:quickChatTerminalExited") return "Terminal exited.";
      if (key === "sidebar:quickChatTerminalExitedWithCode") {
        return `Terminal exited with code ${values?.code}.`;
      }
      if (key === "sidebar:quickChatTerminalError") {
        return `Terminal error: ${values?.error ?? "The terminal could not be started."}`;
      }
      return key;
    },
  }),
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

    return waitFor(() =>
      expect(screen.getByTestId("pty-view-probe").getAttribute("data-lifecycle")).toBe(
        "detach-on-unmount",
      ),
    ).then(() => {
      fireEvent.click(screen.getByTestId("pty-view-probe"));
      expect(startHostShell).toHaveBeenCalledWith({ cols: 80, rows: 24 }, { clientId: tab.tabId });
      view.unmount();
    });
  });

  it("renders accessible lifecycle status for the selected terminal", () => {
    const { rerender } = render(<QuickTerminalTabView tab={tab} onStateChange={vi.fn()} />);

    expect(screen.getByRole("status").textContent).toContain("Connecting to terminal…");

    rerender(
      <QuickTerminalTabView
        tab={{ ...tab, status: "error", error: "Unable to reattach." }}
        onStateChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain("Terminal error: Unable to reattach.");

    rerender(
      <QuickTerminalTabView
        tab={{ ...tab, status: "exited", exitCode: 7 }}
        onStateChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain("Terminal exited with code 7.");
  });
});
