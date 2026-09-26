import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { DevServerPreviewButton } from "./dev-server-preview-button";

const api = vi.hoisted(() => ({
  startProcess: vi.fn(),
  stopProcess: vi.fn(),
}));

const dockview = vi.hoisted(() => ({
  focusOrAddBrowserPanel: vi.fn(),
  addDevServerPanel: vi.fn(),
}));

const appState = vi.hoisted(() => ({
  tasks: { activeSessionId: "session-1" as string | null },
  processes: {
    devProcessBySessionId: {} as Record<string, string>,
    processesById: {} as Record<string, ProcessStatusEntry>,
  },
  upsertProcessStatus: vi.fn(),
  setActiveProcess: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  startProcess: (...args: unknown[]) => api.startProcess(...args),
  stopProcess: (...args: unknown[]) => api.stopProcess(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(appState),
  useAppStoreApi: () => ({ getState: () => appState }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: unknown) => unknown) => selector(dockview),
}));

const START_LABEL = "Start dev server preview";
const STOP_LABEL = "Stop dev server preview";
const SESSION_ID = "session-1";
const DEV_PROCESS_ID = "proc-1";

function setDevProcess(status: string | null) {
  if (status === null) {
    appState.processes.devProcessBySessionId = {};
    appState.processes.processesById = {};
    return;
  }
  appState.processes.devProcessBySessionId = { [SESSION_ID]: DEV_PROCESS_ID };
  appState.processes.processesById = {
    [DEV_PROCESS_ID]: {
      processId: DEV_PROCESS_ID,
      sessionId: SESSION_ID,
      kind: "dev",
      status,
    },
  };
}

function renderButton() {
  return render(<DevServerPreviewButton className="btn" iconClassName="icon" />);
}

describe("DevServerPreviewButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    appState.tasks.activeSessionId = SESSION_ID;
    setDevProcess(null);
    api.startProcess.mockResolvedValue({
      process: {
        id: DEV_PROCESS_ID,
        session_id: SESSION_ID,
        kind: "dev",
        command: "pnpm dev",
        working_dir: "/repo",
        status: "running",
        started_at: "2026-08-16T00:00:00Z",
        updated_at: "2026-08-16T00:00:00Z",
      },
    });
    api.stopProcess.mockResolvedValue(null);
  });

  afterEach(cleanup);

  it("starts the dev process and opens the browser and output panels", async () => {
    renderButton();

    fireEvent.click(screen.getByRole("button", { name: START_LABEL }));

    await waitFor(() => expect(api.startProcess).toHaveBeenCalledWith(SESSION_ID, { kind: "dev" }));
    expect(dockview.focusOrAddBrowserPanel).toHaveBeenCalled();
    expect(dockview.addDevServerPanel).toHaveBeenCalled();
    expect(appState.upsertProcessStatus).toHaveBeenCalledWith(
      expect.objectContaining({ processId: DEV_PROCESS_ID, kind: "dev" }),
    );
  });

  it("issues a single start when both header controls are clicked at once", async () => {
    // The control renders in two dockview headers; each has its own pending
    // flag, so only shared in-flight state can keep the backend's non-atomic
    // dedup from being raced into a second dev process.
    let resolveStart: (value: unknown) => void = () => {};
    api.startProcess.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveStart = resolve;
        }),
    );
    render(
      <>
        <DevServerPreviewButton className="btn" iconClassName="icon" testId="toggle-a" />
        <DevServerPreviewButton className="btn" iconClassName="icon" testId="toggle-b" />
      </>,
    );

    fireEvent.click(screen.getByTestId("toggle-a"));
    fireEvent.click(screen.getByTestId("toggle-b"));
    resolveStart({ process: null });

    await waitFor(() => expect(api.startProcess).toHaveBeenCalledTimes(1));
  });

  it("reuses the preview browser instead of stacking a tab per start", async () => {
    renderButton();

    fireEvent.click(screen.getByRole("button", { name: START_LABEL }));

    await waitFor(() => expect(dockview.focusOrAddBrowserPanel).toHaveBeenCalledTimes(1));
  });

  it("offers Stop while the dev process is running", () => {
    setDevProcess("running");
    renderButton();

    expect(screen.getByRole("button", { name: STOP_LABEL })).toBeTruthy();
    expect(screen.queryByRole("button", { name: START_LABEL })).toBeNull();
  });

  it("treats a starting process as running so the control is never a dead start", () => {
    setDevProcess("starting");
    renderButton();

    expect(screen.getByRole("button", { name: STOP_LABEL })).toBeTruthy();
  });

  it("stops the running dev process instead of starting a second one", async () => {
    setDevProcess("running");
    renderButton();

    fireEvent.click(screen.getByRole("button", { name: STOP_LABEL }));

    await waitFor(() =>
      expect(api.stopProcess).toHaveBeenCalledWith(SESSION_ID, { process_id: DEV_PROCESS_ID }),
    );
    expect(api.startProcess).not.toHaveBeenCalled();
  });

  it("returns to Start once the process reaches a terminal status", () => {
    setDevProcess("exited");
    renderButton();

    expect(screen.getByRole("button", { name: START_LABEL })).toBeTruthy();
  });

  it("is disabled without an active session", () => {
    appState.tasks.activeSessionId = null;
    renderButton();

    expect(screen.getByRole("button", { name: START_LABEL }).hasAttribute("disabled")).toBe(true);
  });
});
