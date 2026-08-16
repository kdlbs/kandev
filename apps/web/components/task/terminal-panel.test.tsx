import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { DEV_SERVER_PANEL_ID } from "@/lib/state/layout-manager/constants";
import { TerminalPanel } from "./terminal-panel";

const SESSION_ID = "session-1";
const DEV_PROCESS_ID = "proc-1";

const appState = vi.hoisted(() => ({
  tasks: { activeSessionId: "session-1" as string | null },
  processes: {
    devProcessBySessionId: {} as Record<string, string>,
    processesById: {} as Record<string, ProcessStatusEntry>,
    outputsByProcessId: {} as Record<string, string>,
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(appState),
}));

vi.mock("@/hooks/use-environment-session-id", () => ({
  useEnvironmentId: () => "env-1",
}));

vi.mock("./task-archived-context", () => ({
  useIsTaskArchived: () => false,
  ArchivedPanelPlaceholder: () => <div data-testid="archived" />,
}));

vi.mock("./shell-terminal", () => ({
  ShellTerminal: ({
    processId,
    processOutput,
  }: {
    processId: string | null;
    processOutput?: string;
  }) => (
    <div data-testid="shell-terminal" data-process-id={processId ?? ""}>
      {processOutput}
    </div>
  ),
}));

vi.mock("./passthrough-terminal", () => ({
  PassthroughTerminal: ({ terminalId }: { terminalId: string }) => (
    <div data-testid="passthrough-terminal" data-terminal-id={terminalId} />
  ),
}));

describe("TerminalPanel", () => {
  beforeEach(() => {
    appState.tasks.activeSessionId = SESSION_ID;
    appState.processes.devProcessBySessionId = {};
    appState.processes.processesById = {};
    appState.processes.outputsByProcessId = {};
  });

  afterEach(cleanup);

  it("renders the live dev-process output, resolving the id from the store", () => {
    appState.processes.devProcessBySessionId = { [SESSION_ID]: DEV_PROCESS_ID };
    appState.processes.outputsByProcessId = { [DEV_PROCESS_ID]: "vite ready on :5173" };

    render(<TerminalPanel panelId={DEV_SERVER_PANEL_ID} params={{ type: DEV_SERVER_PANEL_ID }} />);

    const terminal = screen.getByTestId("shell-terminal");
    expect(terminal.getAttribute("data-process-id")).toBe(DEV_PROCESS_ID);
    expect(terminal.textContent).toContain("vite ready on :5173");
  });

  it("still renders the dev-server view before any process exists", () => {
    render(<TerminalPanel panelId={DEV_SERVER_PANEL_ID} params={{ type: DEV_SERVER_PANEL_ID }} />);

    expect(screen.getByTestId("dev-server-panel")).toBeTruthy();
    expect(screen.queryByTestId("passthrough-terminal")).toBeNull();
  });

  it("ignores a stale processId in the panel params", () => {
    // A restart mints a new process id. A panel that kept the old one in its
    // params would render a dead process: no output, no stopping state.
    appState.processes.devProcessBySessionId = { [SESSION_ID]: DEV_PROCESS_ID };
    appState.processes.outputsByProcessId = {
      [DEV_PROCESS_ID]: "restarted",
      "proc-old": "stale output",
    };

    render(
      <TerminalPanel
        panelId={DEV_SERVER_PANEL_ID}
        params={{ type: DEV_SERVER_PANEL_ID, processId: "proc-old" }}
      />,
    );

    const terminal = screen.getByTestId("shell-terminal");
    expect(terminal.getAttribute("data-process-id")).toBe(DEV_PROCESS_ID);
    expect(terminal.textContent).toBe("restarted");
  });

  it("renders a PTY for ordinary terminal panels", () => {
    render(<TerminalPanel panelId="shell-1" params={{ terminalId: "shell-1" }} />);

    expect(screen.getByTestId("passthrough-terminal").getAttribute("data-terminal-id")).toBe(
      "shell-1",
    );
    expect(screen.queryByTestId("shell-terminal")).toBeNull();
  });
});
