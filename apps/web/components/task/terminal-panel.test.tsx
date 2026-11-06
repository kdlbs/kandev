import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { DEV_SERVER_PANEL_ID } from "@/lib/state/layout-manager/constants";
import { TerminalPanel } from "./terminal-panel";
import { userShellScopeKey } from "@/lib/state/user-shell-scope";

const SESSION_ID = "session-1";
const DEV_PROCESS_ID = "proc-1";
const PASSTHROUGH_TEST_ID = "passthrough-terminal";

const SCOPE_KEY = userShellScopeKey({ environmentId: "env-1", taskId: "task-1" });
const OTHER_TASK_SCOPE_KEY = userShellScopeKey({ environmentId: "env-1", taskId: "task-2" });
const FOREIGN_TERMINAL_ID = "shell-b";

const appState = vi.hoisted(() => ({
  tasks: { activeSessionId: "session-1" as string | null, activeTaskId: "task-1" as string | null },
  userShells: {
    byScope: {} as Record<string, Array<{ terminalId: string; kind: string; state?: string }>>,
    byEnvironmentId: {} as Record<
      string,
      Array<{
        terminalId: string;
        kind?: string;
        state?: string;
        label?: string;
        closable?: boolean;
      }>
    >,
    listed: {} as Record<string, boolean>,
  },
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
    <div data-testid={PASSTHROUGH_TEST_ID} data-terminal-id={terminalId} />
  ),
}));

beforeEach(() => {
  appState.tasks.activeSessionId = SESSION_ID;
  appState.tasks.activeTaskId = "task-1";
  appState.userShells.byScope = {
    [SCOPE_KEY]: [{ terminalId: "shell-1", kind: "ordinary" }],
    [OTHER_TASK_SCOPE_KEY]: [{ terminalId: FOREIGN_TERMINAL_ID, kind: "ordinary" }],
  };
  appState.userShells.byEnvironmentId = {};
  appState.userShells.listed = { [SCOPE_KEY]: true };
  appState.processes.devProcessBySessionId = {};
  appState.processes.processesById = {};
  appState.processes.outputsByProcessId = {};
});

afterEach(cleanup);

describe("TerminalPanel", () => {
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
    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
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

  it("renders a PTY for ordinary terminal panels owned by the current scope", () => {
    render(<TerminalPanel panelId="shell-1" params={{ terminalId: "shell-1" }} />);

    expect(screen.getByTestId(PASSTHROUGH_TEST_ID).getAttribute("data-terminal-id")).toBe(
      "shell-1",
    );
    expect(screen.queryByTestId("shell-terminal")).toBeNull();
  });

  it("never attaches a foreign ordinary terminal", () => {
    render(
      <TerminalPanel
        panelId="shell-foreign"
        params={{ terminalId: "shell-foreign", environmentId: "env-1", taskID: "task-1" }}
      />,
    );

    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
    expect(screen.getByTestId("terminal-panel")).toBeTruthy();
  });

  it("rejects a conflicting task stamp even when the terminal id is listed", () => {
    render(
      <TerminalPanel
        panelId="shell-1"
        params={{ terminalId: "shell-1", environmentId: "env-1", taskID: "task-2" }}
      />,
    );

    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
  });
});

describe("TerminalPanel ownership guards", () => {
  it("rejects a retained task-B panel while task A is active and both caches are populated", () => {
    render(
      <TerminalPanel
        panelId={FOREIGN_TERMINAL_ID}
        params={{
          terminalId: FOREIGN_TERMINAL_ID,
          environmentId: "env-1",
          taskID: "task-2",
        }}
      />,
    );

    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
  });

  it("does not attach a restored panel that references a parked terminal", () => {
    appState.userShells.byScope[SCOPE_KEY] = [
      { terminalId: "shell-parked", kind: "ordinary", state: "parked" },
    ];

    render(<TerminalPanel panelId="shell-parked" params={{ terminalId: "shell-parked" }} />);

    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
  });

  it("still attaches a legacy shell descriptor stored by environment", () => {
    appState.userShells.byEnvironmentId["env-1"] = [
      { terminalId: "shell-legacy", label: "Legacy", closable: false },
    ];

    render(<TerminalPanel panelId="shell-legacy" params={{ terminalId: "shell-legacy" }} />);

    expect(screen.getByTestId(PASSTHROUGH_TEST_ID).getAttribute("data-terminal-id")).toBe(
      "shell-legacy",
    );
  });

  it("rejects a panel stamped for another environment", () => {
    render(
      <TerminalPanel
        panelId="shell-1"
        params={{ terminalId: "shell-1", environmentId: "env-2", taskID: "task-1" }}
      />,
    );

    expect(screen.queryByTestId(PASSTHROUGH_TEST_ID)).toBeNull();
  });
});
