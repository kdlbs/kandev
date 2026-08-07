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
  it("starts a host shell with the terminal tab's stable client id and detaches on unmount", async () => {
    const onStateChange = vi.fn();
    const view = render(<QuickTerminalTabView tab={tab} onStateChange={onStateChange} />);

    await waitFor(() =>
      expect(screen.getByTestId("pty-view-probe").getAttribute("data-lifecycle")).toBe(
        "detach-on-unmount",
      ),
    );

    fireEvent.click(screen.getByTestId("pty-view-probe"));
    expect(startHostShell).toHaveBeenCalledWith({ cols: 80, rows: 24 }, { clientId: tab.tabId });
    await waitFor(() =>
      expect(onStateChange).toHaveBeenCalledWith({
        status: "running",
        sessionId: "session-1",
        exitCode: null,
        error: null,
      }),
    );

    view.unmount();
  });

  it("keeps the shared descriptor when a connecting mount is cancelled before its create resolves", async () => {
    // Under StrictMode the descriptor-create effect runs, tears down, and runs
    // again for the same stable tabId. The idempotent backend create returns the
    // same row, so the cancelled first mount must not delete it — otherwise the
    // live tab (and its bound PTY) is destroyed and cannot survive a reload.
    let resolveCreate: ((value: unknown) => void) | undefined;
    createQuickTerminalTab.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveCreate = resolve;
        }),
    );

    const view = render(<QuickTerminalTabView tab={tab} onStateChange={vi.fn()} />);
    view.unmount();
    resolveCreate?.({
      tabId: tab.tabId,
      workspaceId: tab.workspaceId,
      sessionId: null,
      sequence: 1,
      status: "connecting",
    });
    await Promise.resolve();

    expect(deleteQuickTerminalTab).not.toHaveBeenCalled();
  });

  it("renders accessible lifecycle status for the selected terminal", () => {
    const { rerender } = render(<QuickTerminalTabView tab={tab} onStateChange={vi.fn()} />);

    expect(screen.getByTestId("quick-terminal-status").textContent).toContain(
      "Connecting to terminal…",
    );

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
    expect(screen.getByTestId("quick-terminal-status").textContent).toContain(
      "Terminal exited with code 7.",
    );
  });
});
