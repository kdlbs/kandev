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
  const leaseHints = new Set<string>();
  const changeListeners = new Set<(key: string) => void>();
  const connect = vi.fn(() => vi.fn());
  const state = { status: disabledStatus, progress: emptyProgress };
  const userSettings: {
    lspAutoStartLanguages: string[];
    lspServerConfigs: Record<string, Record<string, unknown>>;
  } = {
    lspAutoStartLanguages: [],
    lspServerConfigs: {},
  };
  let continuityEnabled = false;
  const stop = vi.fn((sessionId: string, language: string) => {
    state.status = disabledStatus;
    for (const listener of changeListeners) listener(`${sessionId}:${language}`);
  });

  return {
    clearEnabledState: vi.fn((sessionId: string, language: string) => {
      const key = `kandev-lsp:${sessionId}:${language}`;
      enabledKeys.delete(key);
      leaseHints.delete(`kandev-lsp-lease:${sessionId}:${language}`);
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
    hasLeaseHint: vi.fn((sessionId: string, language: string) =>
      leaseHints.has(`kandev-lsp-lease:${sessionId}:${language}`),
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
    leaseHints,
    changeListeners,
    stop,
    userSettings,
    get continuityEnabled() {
      return continuityEnabled;
    },
    set continuityEnabled(value: boolean) {
      continuityEnabled = value;
    },
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
    hasLeaseHint: mocks.hasLeaseHint,
    isEnabledInStorage: mocks.isEnabledInStorage,
    onChange: mocks.onChange,
    saveEnabledState: mocks.saveEnabledState,
    stop: mocks.stop,
  },
  toLspLanguage: (language: string) => (language === "typescript" ? language : null),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.continuityEnabled,
}));

import { useLsp, useLspStatus } from "./use-lsp";

const SESSION_ID = "session";
const LANGUAGE = "typescript";
const SERVER_CRASHED_REASON = "server crashed";

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
  mocks.leaseHints.clear();
  mocks.continuityEnabled = false;
  mocks.state.status = mocks.disabledStatus;
  mocks.state.progress = mocks.emptyProgress;
  mocks.userSettings.lspAutoStartLanguages = [];
  mocks.userSettings.lspServerConfigs = {};
});

afterEach(() => {
  mocks.changeListeners.clear();
});

describe("useLsp manual policy leases", () => {
  it("lets a status-only subscriber control the mounted editor lease", async () => {
    const editor = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    const status = renderHook(() => useLspStatus(SESSION_ID, LANGUAGE));

    act(() => status.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());

    act(() => {
      mocks.state.status = {
        state: "error",
        reason: SERVER_CRASHED_REASON,
      } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });
    act(() => status.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));

    act(() => {
      mocks.state.status = { state: "ready" } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });
    act(() => status.result.current.toggle());
    expect(mocks.stop).toHaveBeenCalledOnce();
    expect(mocks.clearEnabledState).toHaveBeenCalledOnce();

    status.unmount();
    expect(mocks.connect).toHaveBeenCalledTimes(2);
    editor.unmount();
  });

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
        reason: SERVER_CRASHED_REASON,
      } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });
    act(() => hook.result.current.toggle());

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));
  });
});

describe("useLsp auto-start policy", () => {
  it("keeps an auto-started server stopped until the user starts it again", async () => {
    const autoStartSession = "auto-start-session";
    mocks.userSettings.lspAutoStartLanguages = [LANGUAGE];
    const hook = renderHook(() => useLsp(autoStartSession, LANGUAGE));

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());
    act(() => {
      mocks.state.status = { state: "ready" } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${autoStartSession}:${LANGUAGE}`);
    });
    act(() => hook.result.current.toggle());

    mocks.userSettings.lspServerConfigs = { [LANGUAGE]: { diagnostics: false } };
    hook.rerender();
    await waitFor(() => {
      expect(mocks.connect).toHaveBeenCalledOnce();
    });

    act(() => hook.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));
  });
});

describe("useLsp browser continuity policy", () => {
  it("reattaches a hinted lease when auto-start is disabled", async () => {
    mocks.continuityEnabled = true;
    mocks.leaseHints.add(`kandev-lsp-lease:${SESSION_ID}:${LANGUAGE}`);

    const hook = renderHook(() => useLsp(SESSION_ID, LANGUAGE));

    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());
    expect(mocks.connect).toHaveBeenCalledWith(SESSION_ID, LANGUAGE, {}, true);
    hook.unmount();
  });

  it("does not restart a crashed lease until the user retries", async () => {
    mocks.continuityEnabled = true;
    const leaseKey = `kandev-lsp-lease:${SESSION_ID}:${LANGUAGE}`;
    const hook = renderHook(() => useLsp(SESSION_ID, LANGUAGE));

    act(() => hook.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());

    act(() => {
      mocks.state.status = { state: "ready" } as typeof mocks.disabledStatus;
      mocks.leaseHints.add(leaseKey);
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(2));

    act(() => {
      mocks.state.status = {
        state: "error",
        reason: SERVER_CRASHED_REASON,
      } as typeof mocks.disabledStatus;
      mocks.leaseHints.delete(leaseKey);
      for (const listener of mocks.changeListeners) listener(`${SESSION_ID}:${LANGUAGE}`);
    });

    expect(mocks.connect).toHaveBeenCalledTimes(2);

    act(() => hook.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledTimes(3));
    hook.unmount();
  });

  it("does not treat a previous session's start as a retry after switching sessions", async () => {
    mocks.continuityEnabled = true;
    const firstSession = "generation-session-a";
    const secondSession = "generation-session-b";
    const hook = renderHook(({ sessionId }) => useLsp(sessionId, LANGUAGE), {
      initialProps: { sessionId: firstSession },
    });

    act(() => hook.result.current.toggle());
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());

    mocks.saveEnabledState(secondSession, LANGUAGE);
    act(() => {
      mocks.state.status = {
        state: "error",
        reason: SERVER_CRASHED_REASON,
      } as typeof mocks.disabledStatus;
      for (const listener of mocks.changeListeners) listener(`${firstSession}:${LANGUAGE}`);
    });

    hook.rerender({ sessionId: secondSession });
    await waitFor(() => expect(mocks.getStatus).toHaveBeenCalled());
    expect(mocks.connect).toHaveBeenCalledOnce();
    hook.unmount();
  });

  it("does not use a stale lease hint when continuity is disabled", async () => {
    mocks.leaseHints.add(`kandev-lsp-lease:${SESSION_ID}:${LANGUAGE}`);

    const hook = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    expect(mocks.connect).not.toHaveBeenCalled();
    hook.unmount();
  });
});

describe("useLsp progress subscription", () => {
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
