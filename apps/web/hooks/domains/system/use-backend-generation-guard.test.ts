import { act, renderHook, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionStatus } from "@/lib/types/connection";

const mocks = vi.hoisted(() => ({
  state: { connection: { status: "connected" as ConnectionStatus } },
  fetchSystemInfo: vi.fn(),
  signalBackendReloadRequired: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/lib/api/domains/system-api", () => ({
  fetchSystemInfo: (...args: unknown[]) => mocks.fetchSystemInfo(...args),
}));
vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  signalBackendReloadRequired: (...args: unknown[]) => mocks.signalBackendReloadRequired(...args),
}));

import { useBackendGenerationGuard } from "./use-backend-generation-guard";

function setBootId(bootId: unknown): void {
  (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__ = {
    runtime: { bootId },
  };
}

function setConnectionStatus(status: ConnectionStatus): void {
  mocks.state.connection.status = status;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

beforeEach(() => {
  setBootId("boot-1");
  setConnectionStatus("connected");
  mocks.fetchSystemInfo.mockReset();
  mocks.signalBackendReloadRequired.mockReset();
});

afterEach(() => {
  delete (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__;
});

describe("useBackendGenerationGuard", () => {
  it("checks the live boot id after the initial connection", async () => {
    mocks.fetchSystemInfo.mockResolvedValue({ boot_id: "boot-1" });

    renderHook(() => useBackendGenerationGuard());

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledWith({ cache: "no-store" }));
    expect(mocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });

  it("checks the live boot id once when StrictMode replays effects", async () => {
    mocks.fetchSystemInfo.mockResolvedValue({ boot_id: "boot-2" });

    renderHook(() => useBackendGenerationGuard(), { wrapper: StrictMode });

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1));
    expect(mocks.signalBackendReloadRequired).toHaveBeenCalledOnce();
  });

  it("does not signal a deferred request after the final StrictMode unmount", async () => {
    const request = deferred<{ boot_id: string }>();
    mocks.fetchSystemInfo.mockReturnValue(request.promise);

    const hook = renderHook(() => useBackendGenerationGuard(), { wrapper: StrictMode });

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1));
    hook.unmount();

    await act(async () => {
      request.resolve({ boot_id: "boot-2" });
      await request.promise;
    });

    expect(mocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });

  it("signals a changed boot id after a reconnect", async () => {
    mocks.fetchSystemInfo.mockResolvedValue({ boot_id: "boot-2" });
    const hook = renderHook(() => useBackendGenerationGuard());

    await waitFor(() =>
      expect(mocks.signalBackendReloadRequired).toHaveBeenCalledWith("boot_id_changed"),
    );
    expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1);

    setConnectionStatus("reconnecting");
    hook.rerender();
    setConnectionStatus("connected");
    hook.rerender();

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(2));
    expect(mocks.signalBackendReloadRequired).toHaveBeenCalledTimes(2);
  });

  it("ignores missing, equal, and failed identities", async () => {
    mocks.fetchSystemInfo
      .mockResolvedValueOnce({ boot_id: "boot-1" })
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error("backend unavailable"));
    const hook = renderHook(() => useBackendGenerationGuard());

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1));
    for (const status of ["reconnecting", "connected", "reconnecting", "connected"] as const) {
      setConnectionStatus(status);
      hook.rerender();
      if (status === "connected") {
        await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalled());
      }
    }

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(3));
    expect(mocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });

  it("ignores an older identity response after a newer connection", async () => {
    const first = deferred<{ boot_id: string }>();
    const second = deferred<{ boot_id: string }>();
    mocks.fetchSystemInfo.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const hook = renderHook(() => useBackendGenerationGuard());

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1));
    setConnectionStatus("reconnecting");
    hook.rerender();
    setConnectionStatus("connected");
    hook.rerender();
    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(2));

    await act(async () => {
      second.resolve({ boot_id: "boot-1" });
      await second.promise;
      first.resolve({ boot_id: "boot-2" });
      await first.promise;
    });

    expect(mocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });

  it("does not compare when the document has no boot id", async () => {
    setBootId(42);
    mocks.fetchSystemInfo.mockResolvedValue({ boot_id: "boot-2" });

    renderHook(() => useBackendGenerationGuard());

    await waitFor(() => expect(mocks.fetchSystemInfo).toHaveBeenCalledTimes(1));
    expect(mocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });
});
