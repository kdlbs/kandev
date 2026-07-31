import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { ActivePlugin } from "./types";

const hostMocks = vi.hoisted(() => ({
  installPluginGlobal: vi.fn(),
  loadPlugins: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("./host", () => ({
  installPluginGlobal: hostMocks.installPluginGlobal,
  loadPlugins: hostMocks.loadPlugins,
}));

import { bootPlugins } from "./boot";

function fakeStore(): StoreApi<AppState> {
  return {} as StoreApi<AppState>;
}

function plugin(id: string): ActivePlugin {
  return { id, name: id, bundleUrl: `/api/plugins/${id}/bundle` };
}

describe("bootPlugins", () => {
  beforeEach(() => {
    hostMocks.installPluginGlobal.mockClear();
    hostMocks.loadPlugins.mockClear();
  });

  afterEach(() => vi.restoreAllMocks());

  it("does nothing when the boot payload has no plugins", () => {
    bootPlugins({ plugins: [] }, fakeStore());
    bootPlugins({}, fakeStore());

    expect(hostMocks.installPluginGlobal).not.toHaveBeenCalled();
    expect(hostMocks.loadPlugins).not.toHaveBeenCalled();
  });

  it("installs the global and loads boot payload plugins", () => {
    const store = fakeStore();
    bootPlugins({ plugins: [plugin("a")] }, store);

    expect(hostMocks.installPluginGlobal).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledWith([plugin("a")], expect.any(Function));
  });

  it("is idempotent across repeated calls for the same store", () => {
    const store = fakeStore();
    const payload = { plugins: [plugin("a")] };

    bootPlugins(payload, store);
    bootPlugins(payload, store);
    bootPlugins(payload, store);

    expect(hostMocks.installPluginGlobal).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(1);
  });

  it("boots independently for distinct store instances", () => {
    const payload = { plugins: [plugin("a")] };

    bootPlugins(payload, fakeStore());
    bootPlugins(payload, fakeStore());

    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(2);
  });
});
