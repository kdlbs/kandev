import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  checkUpdates: vi.fn(),
  fetchUpdates: vi.fn(),
  saveUpdatesChannel: vi.fn(),
  setSystemUpdates: vi.fn(),
  currentUpdates: null as UpdatesResponse | null,
  store: {} as object,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => mocks.store,
  useAppStore: (
    selector: (state: {
      system: { updates: UpdatesResponse | null };
      setSystemUpdates: typeof mocks.setSystemUpdates;
    }) => unknown,
  ) =>
    selector({
      system: { updates: mocks.currentUpdates },
      setSystemUpdates: mocks.setSystemUpdates,
    }),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  checkUpdates: mocks.checkUpdates,
  fetchUpdates: mocks.fetchUpdates,
  saveUpdatesChannel: mocks.saveUpdatesChannel,
}));

import { useUpdates } from "./use-updates";

const SAVE_FAILURE_MESSAGE = "save failed";
const CHECK_FAILURE_MESSAGE = "check failed";
const CHANNEL_SAVE_FAILURE_COPY = "Could not save the update channel. Try again.";

function updates(channel: UpdatesResponse["channel"]): UpdatesResponse {
  const nightly = channel === "nightly";
  return {
    current: "v1.0.0",
    latest: nightly ? "v1.0.1-nightly.shaabcdef123456" : "v1.0.1",
    latest_url: "https://example.test/update",
    latest_checked_at: "2026-08-01T00:00:00Z",
    update_available: true,
    channel,
    channel_editable: true,
    channel_unsupported_reason: "",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  mocks.checkUpdates.mockReset();
  mocks.fetchUpdates.mockReset();
  mocks.saveUpdatesChannel.mockReset();
  mocks.setSystemUpdates.mockReset();
  mocks.currentUpdates = updates("stable");
  mocks.setSystemUpdates.mockImplementation((next: UpdatesResponse) => {
    mocks.currentUpdates = next;
  });
  mocks.store = {
    getState: () => ({ system: { updates: mocks.currentUpdates } }),
  };
});

describe("useUpdates", () => {
  it("keeps checking active until the newest overlapping check settles", async () => {
    const first = deferred<UpdatesResponse>();
    const second = deferred<UpdatesResponse>();
    mocks.checkUpdates.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useUpdates());

    let firstPromise!: Promise<UpdatesResponse | undefined>;
    let secondPromise!: Promise<UpdatesResponse | undefined>;
    act(() => {
      firstPromise = result.current.check();
      secondPromise = result.current.check();
    });
    expect(result.current.isChecking).toBe(true);

    await act(async () => {
      first.resolve(updates("stable"));
      await firstPromise;
    });
    expect(result.current.isChecking).toBe(true);

    await act(async () => {
      second.resolve(updates("nightly"));
      await secondPromise;
    });
    expect(result.current.isChecking).toBe(false);
  });

  it("deduplicates overlapping reloads and publishes their shared result once", async () => {
    const pending = deferred<UpdatesResponse>();
    mocks.fetchUpdates.mockReturnValue(pending.promise);
    const { result } = renderHook(() => useUpdates());

    let firstPromise!: Promise<void>;
    let secondPromise!: Promise<void>;
    act(() => {
      firstPromise = result.current.reload();
      secondPromise = result.current.reload();
    });
    expect(mocks.fetchUpdates).toHaveBeenCalledOnce();

    const nightly = updates("nightly");
    await act(async () => {
      pending.resolve(nightly);
      await Promise.all([firstPromise, secondPromise]);
    });
    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
  });
});

describe("useUpdates request ordering", () => {
  it("starts a fresh reload when a newer failed check makes the shared flight stale", async () => {
    const staleReload = deferred<UpdatesResponse>();
    const recoveryReload = deferred<UpdatesResponse>();
    const checkFailure = new Error(CHECK_FAILURE_MESSAGE);
    mocks.fetchUpdates
      .mockReturnValueOnce(staleReload.promise)
      .mockReturnValueOnce(recoveryReload.promise);
    mocks.checkUpdates.mockRejectedValue(checkFailure);
    const { result } = renderHook(() => useUpdates());

    let stalePromise!: Promise<void>;
    act(() => {
      stalePromise = result.current.reload();
    });
    await act(async () => {
      await expect(result.current.check()).rejects.toBe(checkFailure);
    });

    let recoveryPromise!: Promise<void>;
    act(() => {
      recoveryPromise = result.current.reload();
    });
    expect(mocks.fetchUpdates).toHaveBeenCalledTimes(2);

    const nightly = updates("nightly");
    await act(async () => {
      recoveryReload.resolve(nightly);
      await recoveryPromise;
    });
    await act(async () => {
      staleReload.resolve(updates("stable"));
      await stalePromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
    expect(result.current.error).toBeNull();
  });

  it("suppresses an obsolete check failure after a newer channel save", async () => {
    const pendingCheck = deferred<UpdatesResponse>();
    const nightly = updates("nightly");
    mocks.checkUpdates.mockReturnValue(pendingCheck.promise);
    mocks.saveUpdatesChannel.mockResolvedValue(nightly);
    const { result } = renderHook(() => useUpdates());

    let checkPromise!: Promise<UpdatesResponse | undefined>;
    await act(async () => {
      checkPromise = result.current.check();
      await result.current.saveChannel("nightly");
    });
    await act(async () => {
      pendingCheck.reject(new Error("obsolete check failed"));
      await expect(checkPromise).resolves.toBeUndefined();
    });

    expect(result.current.error).toBeNull();
    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
  });

  it("keeps a save authoritative over an older reload from another hook instance", async () => {
    const pendingReload = deferred<UpdatesResponse>();
    const nightly = updates("nightly");
    mocks.fetchUpdates.mockReturnValue(pendingReload.promise);
    mocks.saveUpdatesChannel.mockResolvedValue(nightly);
    const reader = renderHook(() => useUpdates());
    const writer = renderHook(() => useUpdates());

    let reloadPromise!: Promise<void>;
    await act(async () => {
      reloadPromise = reader.result.current.reload();
      await writer.result.current.saveChannel("nightly");
    });
    await act(async () => {
      pendingReload.resolve(updates("stable"));
      await reloadPromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
  });
});

describe("useUpdates initial load recovery", () => {
  it("does not revalidate when another hook populated the live store", async () => {
    const initialReload = deferred<UpdatesResponse>();
    const pendingCheck = deferred<UpdatesResponse>();
    const checkFailure = new Error(CHECK_FAILURE_MESSAGE);
    mocks.currentUpdates = null;
    mocks.fetchUpdates.mockReturnValue(initialReload.promise);
    mocks.checkUpdates.mockReturnValue(pendingCheck.promise);
    const { result } = renderHook(() => useUpdates());

    let checkPromise!: Promise<UpdatesResponse | undefined>;
    act(() => {
      checkPromise = result.current.check();
    });
    mocks.currentUpdates = updates("stable");
    await act(async () => {
      pendingCheck.reject(checkFailure);
      await expect(checkPromise).rejects.toBe(checkFailure);
    });

    expect(mocks.fetchUpdates).toHaveBeenCalledOnce();
    await act(async () => {
      initialReload.resolve(updates("stable"));
      await initialReload.promise;
    });
  });

  it("revalidates an empty store after a check invalidates the initial reload and fails", async () => {
    const initialReload = deferred<UpdatesResponse>();
    const recoveryReload = deferred<UpdatesResponse>();
    const checkFailure = new Error(CHECK_FAILURE_MESSAGE);
    mocks.currentUpdates = null;
    mocks.fetchUpdates
      .mockReturnValueOnce(initialReload.promise)
      .mockReturnValueOnce(recoveryReload.promise);
    mocks.checkUpdates.mockRejectedValue(checkFailure);
    const { result } = renderHook(() => useUpdates());

    expect(mocks.fetchUpdates).toHaveBeenCalledOnce();
    await act(async () => {
      await expect(result.current.check()).rejects.toBe(checkFailure);
    });
    expect(mocks.fetchUpdates).toHaveBeenCalledTimes(2);

    const stable = updates("stable");
    await act(async () => {
      initialReload.resolve(updates("nightly"));
      recoveryReload.resolve(stable);
      await Promise.all([initialReload.promise, recoveryReload.promise]);
      await Promise.resolve();
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(stable);
    expect(result.current.error).toBe(CHECK_FAILURE_MESSAGE);
  });
});

describe("useUpdates channel saving", () => {
  it("saves a channel and publishes the returned state", async () => {
    const nightly = updates("nightly");
    mocks.saveUpdatesChannel.mockResolvedValue(nightly);
    const { result, rerender } = renderHook(() => useUpdates());

    let response!: UpdatesResponse;
    await act(async () => {
      response = await result.current.saveChannel("nightly");
    });

    expect(mocks.saveUpdatesChannel).toHaveBeenCalledWith("nightly");
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
    expect(response).toBe(nightly);
    rerender();
    expect(result.current.updates).toBe(nightly);
    expect(result.current.error).toBeNull();
  });

  it("surfaces a channel save failure while revalidating the current state", async () => {
    const errorLog = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const failure = new Error(SAVE_FAILURE_MESSAGE);
    const stable = updates("stable");
    mocks.saveUpdatesChannel.mockRejectedValue(failure);
    mocks.fetchUpdates.mockResolvedValue(stable);
    const { result } = renderHook(() => useUpdates());

    await act(async () => {
      await expect(result.current.saveChannel("nightly")).rejects.toBe(failure);
    });

    expect(result.current.error).toBe(CHANNEL_SAVE_FAILURE_COPY);
    expect(errorLog).toHaveBeenCalledWith("[updates] Failed to save update channel", failure);
    expect(mocks.fetchUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(stable);
    errorLog.mockRestore();
  });

  it("revalidates after a failed save suppresses a concurrent reload", async () => {
    const errorLog = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const pendingSave = deferred<UpdatesResponse>();
    const suppressedReload = deferred<UpdatesResponse>();
    const recoveryReload = deferred<UpdatesResponse>();
    const failure = new Error(SAVE_FAILURE_MESSAGE);
    mocks.saveUpdatesChannel.mockReturnValue(pendingSave.promise);
    mocks.fetchUpdates
      .mockReturnValueOnce(suppressedReload.promise)
      .mockReturnValueOnce(recoveryReload.promise);
    const { result } = renderHook(() => useUpdates());

    let savePromise!: Promise<UpdatesResponse>;
    let reloadPromise!: Promise<void>;
    act(() => {
      savePromise = result.current.saveChannel("nightly");
      reloadPromise = result.current.reload();
    });
    await act(async () => {
      suppressedReload.resolve(updates("stable"));
      await reloadPromise;
    });
    expect(mocks.setSystemUpdates).not.toHaveBeenCalled();

    await act(async () => {
      pendingSave.reject(failure);
      await expect(savePromise).rejects.toBe(failure);
    });
    expect(mocks.fetchUpdates).toHaveBeenCalledTimes(2);

    const stable = updates("stable");
    await act(async () => {
      recoveryReload.resolve(stable);
      await recoveryReload.promise;
      await Promise.resolve();
    });
    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(stable);
    expect(result.current.error).toBe(CHANNEL_SAVE_FAILURE_COPY);
    errorLog.mockRestore();
  });
});

describe("useUpdates save/read ordering", () => {
  it("ignores an older check response after a channel save starts", async () => {
    const pendingCheck = deferred<UpdatesResponse>();
    const pendingSave = deferred<UpdatesResponse>();
    mocks.checkUpdates.mockReturnValue(pendingCheck.promise);
    mocks.saveUpdatesChannel.mockReturnValue(pendingSave.promise);
    const { result } = renderHook(() => useUpdates());

    let checkPromise!: Promise<UpdatesResponse | undefined>;
    let savePromise!: Promise<UpdatesResponse>;
    act(() => {
      checkPromise = result.current.check();
      savePromise = result.current.saveChannel("nightly");
    });

    const nightly = updates("nightly");
    await act(async () => {
      pendingSave.resolve(nightly);
      await savePromise;
    });
    await act(async () => {
      pendingCheck.resolve(updates("stable"));
      await checkPromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenLastCalledWith(nightly);
  });

  it("keeps a channel save authoritative over a check started while it is pending", async () => {
    const pendingSave = deferred<UpdatesResponse>();
    const pendingCheck = deferred<UpdatesResponse>();
    mocks.saveUpdatesChannel.mockReturnValue(pendingSave.promise);
    mocks.checkUpdates.mockReturnValue(pendingCheck.promise);
    const { result } = renderHook(() => useUpdates());

    let savePromise!: Promise<UpdatesResponse>;
    let checkPromise!: Promise<UpdatesResponse | undefined>;
    act(() => {
      savePromise = result.current.saveChannel("nightly");
      checkPromise = result.current.check();
    });

    await act(async () => {
      pendingCheck.resolve(updates("stable"));
      await checkPromise;
    });
    expect(mocks.setSystemUpdates).not.toHaveBeenCalled();

    const nightly = updates("nightly");
    await act(async () => {
      pendingSave.resolve(nightly);
      await savePromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
  });
});

describe("useUpdates save serialization", () => {
  it("keeps the newest save authoritative across hook instances", async () => {
    const firstSave = deferred<UpdatesResponse>();
    const secondSave = deferred<UpdatesResponse>();
    mocks.saveUpdatesChannel
      .mockReturnValueOnce(firstSave.promise)
      .mockReturnValueOnce(secondSave.promise);
    const first = renderHook(() => useUpdates());
    const second = renderHook(() => useUpdates());

    let firstPromise!: Promise<UpdatesResponse>;
    let secondPromise!: Promise<UpdatesResponse>;
    await act(async () => {
      firstPromise = first.result.current.saveChannel("nightly");
      secondPromise = second.result.current.saveChannel("stable");
      await Promise.resolve();
    });
    expect(mocks.saveUpdatesChannel).toHaveBeenCalledOnce();
    expect(mocks.saveUpdatesChannel).toHaveBeenCalledWith("nightly");

    await act(async () => {
      firstSave.resolve(updates("nightly"));
      await firstPromise;
    });
    expect(mocks.saveUpdatesChannel).toHaveBeenCalledTimes(2);
    expect(mocks.saveUpdatesChannel).toHaveBeenLastCalledWith("stable");

    const stable = updates("stable");
    await act(async () => {
      secondSave.resolve(stable);
      await secondPromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(stable);
  });
});
