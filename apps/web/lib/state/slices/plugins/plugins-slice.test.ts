import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createPluginsSlice } from "./plugins-slice";
import type { PluginsSlice } from "./types";
import type { PluginRecord } from "@/lib/types/plugins";

function makeStore() {
  return create<PluginsSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createPluginsSlice as any)(...a) })),
  );
}

function plugin(id: string, overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id,
    api_version: 1,
    version: "1.0.0",
    display_name: `Plugin ${id}`,
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "registered",
    install_path: `/home/user/.kandev/plugins/${id}/1.0.0`,
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

describe("plugins slice", () => {
  it("starts empty, not loading, not loaded, no error", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.plugins.items).toEqual([]);
    expect(s.plugins.loading).toBe(false);
    expect(s.plugins.loaded).toBe(false);
    expect(s.plugins.error).toBeNull();
  });

  it("setPlugins replaces items, flips loaded=true, and clears error", () => {
    const store = makeStore();
    store.getState().setPluginsError("boom");
    store.getState().setPlugins([plugin("a"), plugin("b")]);
    const s = store.getState();
    expect(s.plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
    expect(s.plugins.loaded).toBe(true);
    expect(s.plugins.error).toBeNull();
  });

  it("setPluginsLoading toggles the loading flag", () => {
    const store = makeStore();
    store.getState().setPluginsLoading(true);
    expect(store.getState().plugins.loading).toBe(true);
    store.getState().setPluginsLoading(false);
    expect(store.getState().plugins.loading).toBe(false);
  });

  it("setPluginsError sets the error message", () => {
    const store = makeStore();
    store.getState().setPluginsError("network error");
    expect(store.getState().plugins.error).toBe("network error");
    store.getState().setPluginsError(null);
    expect(store.getState().plugins.error).toBeNull();
  });

  it("upsertPlugin appends a new plugin", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a")]);
    store.getState().upsertPlugin(plugin("b"));
    expect(store.getState().plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
  });

  it("upsertPlugin replaces an existing plugin by id in place", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a", { status: "registered" }), plugin("b")]);
    store.getState().upsertPlugin(plugin("a", { status: "active" }));
    const s = store.getState();
    expect(s.plugins.items.map((p) => p.id)).toEqual(["a", "b"]);
    expect(s.plugins.items[0].status).toBe("active");
  });

  it("removePlugin filters by id", () => {
    const store = makeStore();
    store.getState().setPlugins([plugin("a"), plugin("b"), plugin("c")]);
    store.getState().removePlugin("b");
    expect(store.getState().plugins.items.map((p) => p.id)).toEqual(["a", "c"]);
  });
});
