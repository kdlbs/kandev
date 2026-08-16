import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { useDevServerPreview } from "./use-dev-server-preview";

const SESSION_ID = "session-1";
const DEV_PROCESS_ID = "proc-1";

const api = vi.hoisted(() => ({
  startProcess: vi.fn(),
  stopProcess: vi.fn(),
}));

const appState = vi.hoisted(() => ({
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

function startResponse(status: string, updatedAt: string) {
  return {
    process: {
      id: DEV_PROCESS_ID,
      session_id: SESSION_ID,
      kind: "dev",
      command: "pnpm dev",
      working_dir: "/repo",
      status,
      started_at: "2026-08-16T10:00:00Z",
      updated_at: updatedAt,
    },
  };
}

describe("useDevServerPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    appState.processes.devProcessBySessionId = {};
    appState.processes.processesById = {};
    api.stopProcess.mockResolvedValue(null);
  });

  afterEach(cleanup);

  it("seeds the store from the start response", async () => {
    api.startProcess.mockResolvedValue(startResponse("running", "2026-08-16T10:00:01Z"));
    const { result } = renderHook(() => useDevServerPreview(SESSION_ID));

    await act(() => result.current.start());

    expect(appState.upsertProcessStatus).toHaveBeenCalledWith(
      expect.objectContaining({ processId: DEV_PROCESS_ID, status: "running" }),
    );
    expect(appState.setActiveProcess).toHaveBeenCalledWith(SESSION_ID, DEV_PROCESS_ID);
  });

  it("does not overwrite a terminal status that arrived before the response", async () => {
    // A dev script that fails immediately can have its `exited` WebSocket frame
    // land while the start POST is still open. Writing the response's older
    // `running` over it would strand the control on Stop with nothing to stop.
    let resolveStart: (value: unknown) => void = () => {};
    api.startProcess.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveStart = resolve;
        }),
    );
    const { result } = renderHook(() => useDevServerPreview(SESSION_ID));

    const pending = act(() => result.current.start());
    appState.processes.devProcessBySessionId = { [SESSION_ID]: DEV_PROCESS_ID };
    appState.processes.processesById = {
      [DEV_PROCESS_ID]: {
        processId: DEV_PROCESS_ID,
        sessionId: SESSION_ID,
        kind: "dev",
        status: "exited",
        updatedAt: "2026-08-16T10:00:05Z",
      },
    };
    resolveStart(startResponse("running", "2026-08-16T10:00:01Z"));
    await pending;

    expect(appState.upsertProcessStatus).not.toHaveBeenCalled();
  });

  it("applies the response when the store holds an older frame for the same process", async () => {
    appState.processes.processesById = {
      [DEV_PROCESS_ID]: {
        processId: DEV_PROCESS_ID,
        sessionId: SESSION_ID,
        kind: "dev",
        status: "starting",
        updatedAt: "2026-08-16T10:00:00Z",
      },
    };
    api.startProcess.mockResolvedValue(startResponse("running", "2026-08-16T10:00:02Z"));
    const { result } = renderHook(() => useDevServerPreview(SESSION_ID));

    await act(() => result.current.start());

    expect(appState.upsertProcessStatus).toHaveBeenCalledWith(
      expect.objectContaining({ status: "running" }),
    );
  });

  it("collapses concurrent starts for one session into a single request", async () => {
    let resolveStart: (value: unknown) => void = () => {};
    api.startProcess.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveStart = resolve;
        }),
    );
    const first = renderHook(() => useDevServerPreview(SESSION_ID));
    const second = renderHook(() => useDevServerPreview(SESSION_ID));

    const pending = act(async () => {
      void first.result.current.start();
      void second.result.current.start();
      resolveStart(startResponse("running", "2026-08-16T10:00:01Z"));
    });
    await pending;

    await waitFor(() => expect(api.startProcess).toHaveBeenCalledTimes(1));
  });

  it("stops the session's dev process", async () => {
    appState.processes.devProcessBySessionId = { [SESSION_ID]: DEV_PROCESS_ID };
    const { result } = renderHook(() => useDevServerPreview(SESSION_ID));

    await act(() => result.current.stop());

    expect(api.stopProcess).toHaveBeenCalledWith(SESSION_ID, { process_id: DEV_PROCESS_ID });
  });

  it("does nothing without a session", async () => {
    const { result } = renderHook(() => useDevServerPreview(null));

    await act(() => result.current.start());
    await act(() => result.current.stop());

    expect(api.startProcess).not.toHaveBeenCalled();
    expect(api.stopProcess).not.toHaveBeenCalled();
  });
});
