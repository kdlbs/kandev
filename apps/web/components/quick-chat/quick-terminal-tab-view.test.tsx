import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";

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
      data-testid={PTY_VIEW_PROBE}
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

const PTY_VIEW_PROBE = "pty-view-probe";

it("starts a host shell with the terminal tab's stable client id and detaches on unmount", async () => {
  const onStateChange = vi.fn();
  const view = render(<QuickTerminalTabView tab={tab} onStateChange={onStateChange} />);

  await waitFor(() =>
    expect(screen.getByTestId(PTY_VIEW_PROBE).getAttribute("data-lifecycle")).toBe(
      "detach-on-unmount",
    ),
  );

  fireEvent.click(screen.getByTestId(PTY_VIEW_PROBE));
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

it("continues a local start when a concurrent resync marked its descriptor unavailable", async () => {
  const onDescriptorReady = vi.fn();
  createQuickTerminalTab.mockResolvedValueOnce({
    ...tab,
    status: "exited",
    error: "terminal session unavailable",
  });

  const view = render(
    <QuickTerminalTabView
      tab={tab}
      onStateChange={vi.fn()}
      onDescriptorReady={onDescriptorReady}
    />,
  );

  await waitFor(() => expect(onDescriptorReady).toHaveBeenCalledWith({ ...tab }));
  expect(screen.getByTestId(PTY_VIEW_PROBE)).toBeTruthy();
  view.unmount();
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

it.each([
  ["exited", { ...tab, sessionId: "old-session", status: "exited" as const }],
  ["unavailable", { ...tab, status: "error" as const, error: "terminal session unavailable" }],
])(
  "offers explicit same-tab recovery for a %s terminal without starting on mount",
  async (_, lostTab) => {
    startHostShell.mockClear();
    createQuickTerminalTab.mockClear();
    const onRecover = vi.fn();

    const view = render(
      <QuickTerminalTabView tab={lostTab} onStateChange={vi.fn()} onRecover={onRecover} />,
    );
    const tabPanel = within(view.container);

    await waitFor(() =>
      expect(tabPanel.getByRole("button", { name: "Start new shell" })).toBeTruthy(),
    );
    expect(startHostShell).not.toHaveBeenCalled();
    expect(createQuickTerminalTab).not.toHaveBeenCalled();
    fireEvent.click(tabPanel.getByRole("button", { name: "Start new shell" }));
    expect(onRecover).toHaveBeenCalledOnce();

    view.unmount();
  },
);

it("does not create a descriptor or mount a PTY while the tab is closing", async () => {
  createQuickTerminalTab.mockClear();
  startHostShell.mockClear();

  const view = render(
    <QuickTerminalTabView
      tab={tab}
      onStateChange={vi.fn()}
      recoveryGeneration={1}
      recoveryClosing
    />,
  );

  await Promise.resolve();
  expect(createQuickTerminalTab).not.toHaveBeenCalled();
  expect(screen.queryByTestId(PTY_VIEW_PROBE)).toBeNull();
  expect(startHostShell).not.toHaveBeenCalled();
  view.unmount();
});
