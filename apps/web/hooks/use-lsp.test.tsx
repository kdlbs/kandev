import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const disabledStatus = { state: "disabled" } as { state: string; reason?: string };
  const emptyProgress = {
    initializingSince: null as number | null,
    active: [],
    completed: null,
    hasReportedProgress: false,
  };
  const enabledKeys = new Set<string>();
  const changeListeners = new Set<(key: string) => void>();
  const connect = vi.fn(() => vi.fn());
  const state = { status: disabledStatus, progress: emptyProgress };
  const userSettings = {
    lspAutoStartLanguages: [],
    lspServerConfigs: {},
  };

  return {
    clearEnabledState: vi.fn((sessionId: string, language: string) => {
      const key = `kandev-lsp:${sessionId}:${language}`;
      enabledKeys.delete(key);
      for (const listener of changeListeners) listener(`${sessionId}:${language}`);
    }),
    connect,
    disabledStatus,
    emptyProgress,
    enabledKeys,
    getProgress: vi.fn(() => state.progress),
    getStatus: vi.fn(() => state.status),
    isEnabledInStorage: vi.fn((sessionId: string, language: string) =>
      enabledKeys.has(`kandev-lsp:${sessionId}:${language}`),
    ),
    onChange: vi.fn((listener: (key: string) => void) => {
      changeListeners.add(listener);
      return () => changeListeners.delete(listener);
    }),
    saveEnabledState: vi.fn((sessionId: string, language: string) => {
      const key = `kandev-lsp:${sessionId}:${language}`;
      enabledKeys.add(key);
      for (const listener of changeListeners) listener(`${sessionId}:${language}`);
    }),
    state,
    changeListeners,
    stop: vi.fn(),
    userSettings,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { userSettings: Record<string, unknown> }) => unknown) =>
    selector({ userSettings: mocks.userSettings }),
}));

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    clearEnabledState: mocks.clearEnabledState,
    connect: mocks.connect,
    getProgress: mocks.getProgress,
    getStatus: mocks.getStatus,
    isEnabledInStorage: mocks.isEnabledInStorage,
    onChange: mocks.onChange,
    saveEnabledState: mocks.saveEnabledState,
    stop: mocks.stop,
  },
  toLspLanguage: (language: string) => (language === "typescript" ? language : null),
}));

import { useLsp } from "./use-lsp";

const SESSION_ID = "session";
const LANGUAGE = "typescript";

beforeEach(() => {
  mocks.connect.mockClear();
  mocks.clearEnabledState.mockClear();
  mocks.isEnabledInStorage.mockClear();
  mocks.getProgress.mockClear();
  mocks.getStatus.mockClear();
  mocks.onChange.mockClear();
  mocks.saveEnabledState.mockClear();
  mocks.stop.mockClear();
  mocks.enabledKeys.clear();
  mocks.state.status = mocks.disabledStatus;
  mocks.state.progress = mocks.emptyProgress;
});

afterEach(() => {
  mocks.changeListeners.clear();
});

describe("useLsp manual policy leases", () => {
  it("gives every mounted matching editor a lease when manually enabled", async () => {
    const first = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    const second = renderHook(() => useLsp(SESSION_ID, LANGUAGE));

    act(() => first.result.current.toggle());

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));
    const firstRelease = mocks.connect.mock.results[0]?.value as ReturnType<typeof vi.fn>;
    const secondRelease = mocks.connect.mock.results[1]?.value as ReturnType<typeof vi.fn>;

    first.unmount();
    expect(firstRelease).toHaveBeenCalledOnce();
    expect(secondRelease).not.toHaveBeenCalled();

    second.unmount();
    expect(secondRelease).toHaveBeenCalledOnce();
  });

  it("restores a saved policy only through mounted editor leases", async () => {
    mocks.saveEnabledState(SESSION_ID, LANGUAGE);
    expect(mocks.connect).not.toHaveBeenCalled();

    const first = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    const second = renderHook(() => useLsp(SESSION_ID, LANGUAGE));

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));
    const firstRelease = mocks.connect.mock.results[0]?.value as ReturnType<typeof vi.fn>;
    const secondRelease = mocks.connect.mock.results[1]?.value as ReturnType<typeof vi.fn>;

    first.unmount();
    expect(firstRelease).toHaveBeenCalledOnce();
    expect(secondRelease).not.toHaveBeenCalled();

    second.unmount();
    expect(secondRelease).toHaveBeenCalledOnce();
  });

  it("retries a failed manually enabled connection in the mounted editor", async () => {
    const hook = renderHook(() => useLsp(SESSION_ID, LANGUAGE));

    act(() => hook.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());

    act(() => {
      mocks.state.status = {
        state: "error",
        reason: "server crashed",
      } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });
    act(() => hook.result.current.toggle());

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));
  });

  it("subscribes to the current connection progress snapshot", () => {
    const hook = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    const progress = {
      initializingSince: 100,
      active: [],
      completed: null,
      hasReportedProgress: false,
    };

    act(() => {
      mocks.state.progress = progress;
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });

    expect((hook.result.current as unknown as { progress?: typeof progress }).progress).toBe(
      progress,
    );
  });
});
