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

  it("does nothing when the plugins feature flag is off", () => {
    bootPlugins({ plugins: [plugin("a")] }, fakeStore(), false);

    expect(hostMocks.installPluginGlobal).not.toHaveBeenCalled();
    expect(hostMocks.loadPlugins).not.toHaveBeenCalled();
  });

  it("does nothing when the boot payload has no plugins", () => {
    bootPlugins({ plugins: [] }, fakeStore(), true);
    bootPlugins({}, fakeStore(), true);

    expect(hostMocks.installPluginGlobal).not.toHaveBeenCalled();
    expect(hostMocks.loadPlugins).not.toHaveBeenCalled();
  });

  it("installs the global and loads boot payload plugins once enabled", () => {
    const store = fakeStore();
    bootPlugins({ plugins: [plugin("a")] }, store, true);

    expect(hostMocks.installPluginGlobal).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledWith([plugin("a")], expect.any(Function));
  });

  it("is idempotent across repeated calls for the same store", () => {
    const store = fakeStore();
    const payload = { plugins: [plugin("a")] };

    bootPlugins(payload, store, true);
    bootPlugins(payload, store, true);
    bootPlugins(payload, store, true);

    expect(hostMocks.installPluginGlobal).toHaveBeenCalledTimes(1);
    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(1);
  });

  it("boots independently for distinct store instances", () => {
    const payload = { plugins: [plugin("a")] };

    bootPlugins(payload, fakeStore(), true);
    bootPlugins(payload, fakeStore(), true);

    expect(hostMocks.loadPlugins).toHaveBeenCalledTimes(2);
  });
});
