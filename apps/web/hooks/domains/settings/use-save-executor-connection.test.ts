import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import type { Executor } from "@/lib/types/http";
import { useSaveExecutorConnection } from "./use-save-executor-connection";

const mocks = vi.hoisted(() => ({
  updateExecutor: vi.fn(),
  listExecutors: vi.fn(),
  setExecutors: vi.fn(),
  items: [] as Executor[],
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateExecutor: (...args: unknown[]) => mocks.updateExecutor(...args),
  listExecutors: (...args: unknown[]) => mocks.listExecutors(...args),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ executors: { items: mocks.items }, setExecutors: mocks.setExecutors }),
  }),
}));

const form = { name: "Renamed", identity_source: "agent" } as SSHExecutorConfig;
const buildConfig = () => ({ ssh_host: "box.lan", ssh_host_fingerprint: "SHA256:new" });

function executor(id: string, fingerprint: string): Executor {
  return { id, name: id, config: { ssh_host_fingerprint: fingerprint } } as unknown as Executor;
}

describe("useSaveExecutorConnection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.items = [executor("e1", "SHA256:old"), executor("e2", "SHA256:other")];
    mocks.updateExecutor.mockResolvedValue(undefined);
  });

  it("saves, then replaces the store's executors with a fresh list", async () => {
    const fresh = [executor("e1", "SHA256:new")];
    mocks.listExecutors.mockResolvedValue({ executors: fresh });
    const onSaved = vi.fn();
    const { result } = renderHook(() => useSaveExecutorConnection("e1", buildConfig, onSaved));

    await act(() => result.current(form));

    expect(mocks.updateExecutor).toHaveBeenCalledWith("e1", {
      name: "Renamed",
      config: buildConfig(),
    });
    expect(mocks.setExecutors).toHaveBeenCalledWith(fresh);
    expect(onSaved).toHaveBeenCalledTimes(1);
  });

  it("patches the saved executor in place when the refresh fails", async () => {
    mocks.listExecutors.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useSaveExecutorConnection("e1", buildConfig));

    await act(() => result.current(form));

    const [written] = mocks.setExecutors.mock.calls[0] as [Executor[]];
    expect(written[0]).toMatchObject({ id: "e1", name: "Renamed", config: buildConfig() });
    expect(written[1]).toBe(mocks.items[1]);
  });

  it("rejects without touching the store when the save fails", async () => {
    mocks.updateExecutor.mockRejectedValue(new Error("forbidden"));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useSaveExecutorConnection("e1", buildConfig, onSaved));

    await expect(result.current(form)).rejects.toThrow("forbidden");
    expect(mocks.setExecutors).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });
});
