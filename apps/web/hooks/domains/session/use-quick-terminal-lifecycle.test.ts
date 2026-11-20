import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";

const mockListQuickTerminalTabs = vi.fn();
const mockUpdateQuickTerminalTab = vi.fn();

vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  listQuickTerminalTabs: (...args: unknown[]) => mockListQuickTerminalTabs(...args),
  updateQuickTerminalTab: (...args: unknown[]) => mockUpdateQuickTerminalTab(...args),
  toQuickTerminalTab: (tab: unknown) => tab,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { useQuickTerminalLifecycle } from "./use-quick-terminal-lifecycle";

const WORKSPACE_ID = "workspace-1";
const TAB_ID = "terminal-1";
const OLD_SESSION_ID = "old-session";
const REMOTE_SESSION_ID = "remote-session";

function terminal(overrides: Partial<QuickTerminalTab> = {}): QuickTerminalTab {
  return {
    tabId: TAB_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: OLD_SESSION_ID,
    sequence: 3,
    status: "exited",
    ...overrides,
  };
}

function makeStore(tab = terminal()) {
  return {
    terminalTabs: [tab],
    updateQuickTerminal: vi.fn(),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListQuickTerminalTabs.mockResolvedValue({ tabs: [] });
  mockUpdateQuickTerminalTab.mockResolvedValue({
    tabId: TAB_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: null,
    sequence: 3,
    status: "exited",
  });
});

it("reattaches to a live shell another client already bound to the same descriptor", async () => {
  const store = makeStore();
  mockListQuickTerminalTabs.mockResolvedValueOnce({
    tabs: [terminal({ sessionId: REMOTE_SESSION_ID, status: "running", exitCode: undefined })],
  });
  const { result } = renderHook(() => useQuickTerminalLifecycle(WORKSPACE_ID, store));

  await act(async () => result.current.handleRecoverTerminal(TAB_ID));

  expect(mockListQuickTerminalTabs).toHaveBeenCalledWith(WORKSPACE_ID);
  expect(store.updateQuickTerminal).toHaveBeenCalledWith(TAB_ID, {
    sequence: 3,
    sessionId: REMOTE_SESSION_ID,
    status: "running",
    exitCode: undefined,
    error: undefined,
  });
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({ generation: 1 });

  act(() => {
    result.current.handleTerminalStateChange(
      TAB_ID,
      { status: "exited", sessionId: REMOTE_SESSION_ID, exitCode: 0, error: null },
      1,
    );
  });
  await waitFor(() =>
    expect(mockUpdateQuickTerminalTab).toHaveBeenCalledWith(TAB_ID, {
      sessionId: null,
      status: "exited",
      exitCode: 0,
      error: null,
    }),
  );
});

it("coalesces concurrent activation and permits an explicit retry after preparation fails", async () => {
  const store = makeStore();
  const pendingList = deferred<{ tabs: QuickTerminalTab[] }>();
  mockListQuickTerminalTabs
    .mockImplementationOnce(() => pendingList.promise)
    .mockRejectedValueOnce(new Error("descriptor read failed"))
    .mockResolvedValueOnce({ tabs: [terminal({ sessionId: null, status: "exited" })] });
  const { result } = renderHook(() => useQuickTerminalLifecycle(WORKSPACE_ID, store));

  let first!: Promise<void>;
  let second!: Promise<void>;
  act(() => {
    first = result.current.handleRecoverTerminal(TAB_ID);
    second = result.current.handleRecoverTerminal(TAB_ID);
  });
  expect(first).toBe(second);
  await waitFor(() => expect(mockListQuickTerminalTabs).toHaveBeenCalledTimes(1));
  pendingList.resolve({ tabs: [terminal({ sessionId: null, status: "exited" })] });
  await act(async () => first);
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({
    generation: 1,
    phase: "starting",
  });

  act(() => {
    result.current.handleTerminalStateChange(
      TAB_ID,
      { status: "error", sessionId: null, exitCode: null, error: "start failed" },
      1,
    );
  });
  await waitFor(() =>
    expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({ generation: 1 }),
  );

  await act(async () => result.current.handleRecoverTerminal(TAB_ID));
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({ generation: 2 });
  expect(store.updateQuickTerminal).toHaveBeenCalledWith(
    TAB_ID,
    expect.objectContaining({ status: "error", error: "descriptor read failed" }),
  );

  await act(async () => result.current.handleRecoverTerminal(TAB_ID));
  expect(mockListQuickTerminalTabs).toHaveBeenCalledTimes(3);
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({
    generation: 3,
    phase: "starting",
  });
});

it("waits for an issued lifecycle write before reading the descriptor and ignores retired callbacks", async () => {
  const store = makeStore();
  const pendingWrite = deferred<QuickTerminalTab>();
  mockUpdateQuickTerminalTab.mockReturnValueOnce(pendingWrite.promise);
  mockListQuickTerminalTabs.mockResolvedValueOnce({
    tabs: [terminal({ sessionId: null, status: "exited" })],
  });
  const { result } = renderHook(() => useQuickTerminalLifecycle(WORKSPACE_ID, store));

  act(() => {
    result.current.handleTerminalStateChange(
      TAB_ID,
      { status: "exited", sessionId: OLD_SESSION_ID, exitCode: 0, error: null },
      0,
    );
  });
  await waitFor(() => expect(mockUpdateQuickTerminalTab).toHaveBeenCalledTimes(1));

  let recovery!: Promise<void>;
  act(() => {
    recovery = result.current.handleRecoverTerminal(TAB_ID);
    result.current.handleTerminalStateChange(
      TAB_ID,
      { status: "exited", sessionId: OLD_SESSION_ID, exitCode: 0, error: null },
      0,
    );
  });
  expect(mockListQuickTerminalTabs).not.toHaveBeenCalled();
  expect(mockUpdateQuickTerminalTab).toHaveBeenCalledTimes(1);

  pendingWrite.resolve(terminal({ sessionId: null, status: "exited" }));
  await act(async () => recovery);

  expect(mockListQuickTerminalTabs).toHaveBeenCalledTimes(1);
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({
    generation: 1,
    phase: "starting",
  });
  expect(store.updateQuickTerminal).not.toHaveBeenCalledWith(
    TAB_ID,
    expect.objectContaining({ sequence: 3, sessionId: OLD_SESSION_ID, status: "exited" }),
  );
});

it("cancels preparation when the tab is explicitly closed", async () => {
  const store = makeStore();
  const pendingList = deferred<{ tabs: QuickTerminalTab[] }>();
  mockListQuickTerminalTabs.mockImplementationOnce(() => pendingList.promise);
  const { result } = renderHook(() => useQuickTerminalLifecycle(WORKSPACE_ID, store));

  let recovery!: Promise<void>;
  act(() => {
    recovery = result.current.handleRecoverTerminal(TAB_ID);
  });
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({
    generation: 1,
    phase: "preparing",
  });
  act(() => {
    store.terminalTabs = [];
    result.current.beginClose(TAB_ID);
    result.current.finishClose(TAB_ID, true);
  });
  pendingList.resolve({ tabs: [terminal({ sessionId: null, status: "exited" })] });
  await act(async () => recovery);

  expect(store.updateQuickTerminal).toHaveBeenCalledTimes(1);
  expect(result.current.terminalRecoveryByTab[TAB_ID]).toEqual({
    generation: 2,
    closing: true,
  });
});
