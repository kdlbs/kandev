import { afterEach, describe, expect, it, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createPluginsSlice } from "./plugins-slice";
import type { PluginsSlice } from "./types";
import type { PluginRecord } from "@/lib/types/plugins";
import { verifyPluginPublisher } from "@/lib/api/domains/plugins-api";

vi.mock("@/lib/api/domains/plugins-api", () => ({
  verifyPluginPublisher: vi.fn(),
}));

const verifyPluginPublisherMock = vi.mocked(verifyPluginPublisher);

afterEach(() => {
  vi.clearAllMocks();
});

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

const INSTALLED_VERSION = "1.0.0";
const INSTALLED_ID = "installation-a";
const REPLACEMENT_VERSION = "2.0.0";
const REPLACEMENT_ID = "installation-b";

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

describe("plugins slice publisher updates", () => {
  it("updates only publisher fields for the matching installation snapshot", () => {
    const store = makeStore();
    store.getState().setPlugins([
      plugin("a", {
        version: INSTALLED_VERSION,
        installation_id: INSTALLED_ID,
        status: "active",
        auto_update: false,
      }),
    ]);
    const applied = store
      .getState()
      .updatePluginPublisher(
        "a",
        INSTALLED_ID,
        INSTALLED_VERSION,
        { status: "verified", login: "acme" },
        { origin: "upload", package_id: "a", version: INSTALLED_VERSION },
      );
    expect(applied).toBe(true);
    expect(store.getState().plugins.items[0]).toMatchObject({
      status: "active",
      auto_update: false,
      publisher_identity: { status: "verified", login: "acme" },
    });
  });

  it("ignores delayed publisher responses for an uninstall or replacement", () => {
    const store = makeStore();
    store
      .getState()
      .setPlugins([plugin("a", { version: INSTALLED_VERSION, installation_id: INSTALLED_ID })]);
    store.getState().removePlugin("a");
    expect(
      store
        .getState()
        .updatePluginPublisher(
          "a",
          INSTALLED_ID,
          INSTALLED_VERSION,
          { status: "verified", login: "acme" },
          { origin: "upload", package_id: "a", version: INSTALLED_VERSION },
        ),
    ).toBe(false);
    expect(store.getState().plugins.items).toHaveLength(0);

    store.getState().setPlugins([
      plugin("a", {
        version: REPLACEMENT_VERSION,
        installation_id: REPLACEMENT_ID,
        status: "active",
      }),
    ]);
    expect(
      store
        .getState()
        .updatePluginPublisher(
          "a",
          INSTALLED_ID,
          INSTALLED_VERSION,
          { status: "verified", login: "acme" },
          { origin: "upload", package_id: "a", version: INSTALLED_VERSION },
        ),
    ).toBe(false);
    expect(store.getState().plugins.items[0]).toMatchObject({
      version: REPLACEMENT_VERSION,
      installation_id: REPLACEMENT_ID,
      status: "active",
    });
  });
});

describe("plugins slice async publisher verification", () => {
  it("verifies through the slice and never reinserts a removed plugin", async () => {
    const store = makeStore();
    let resolveResponse!: (record: PluginRecord) => void;
    const pending = new Promise<PluginRecord>((resolve) => {
      resolveResponse = resolve;
    });
    verifyPluginPublisherMock.mockReturnValueOnce(pending);
    store
      .getState()
      .setPlugins([plugin("a", { version: INSTALLED_VERSION, installation_id: INSTALLED_ID })]);
    const resultPromise = store
      .getState()
      .verifyPluginPublisher("a", INSTALLED_ID, INSTALLED_VERSION);
    store.getState().removePlugin("a");
    resolveResponse(
      plugin("a", {
        version: INSTALLED_VERSION,
        installation_id: INSTALLED_ID,
        publisher_identity: { status: "verified", login: "acme" },
      }),
    );
    await expect(resultPromise).resolves.toBe(false);
    expect(store.getState().plugins.items).toHaveLength(0);
  });

  it("ignores a verification response after a replacement", async () => {
    const store = makeStore();
    store
      .getState()
      .setPlugins([plugin("a", { version: INSTALLED_VERSION, installation_id: INSTALLED_ID })]);
    verifyPluginPublisherMock.mockResolvedValueOnce(
      plugin("a", {
        version: INSTALLED_VERSION,
        installation_id: INSTALLED_ID,
        publisher_identity: { status: "verified", login: "acme" },
      }),
    );
    const resultPromise = store
      .getState()
      .verifyPluginPublisher("a", INSTALLED_ID, INSTALLED_VERSION);
    store.getState().setPlugins([
      plugin("a", {
        version: REPLACEMENT_VERSION,
        installation_id: REPLACEMENT_ID,
        status: "active",
      }),
    ]);
    await expect(resultPromise).resolves.toBe(false);
    expect(store.getState().plugins.items[0]).toMatchObject({
      version: REPLACEMENT_VERSION,
      installation_id: REPLACEMENT_ID,
      status: "active",
    });
  });
});
