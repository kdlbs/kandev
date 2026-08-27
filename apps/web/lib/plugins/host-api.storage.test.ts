import { afterEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";

const NOTES_PLUGIN_ID = "kandev-plugin-notes";
const TEST_UPDATED_AT = "2026-01-01T00:00:00Z";
const TASK_ID = "task_1";
const NOTE_KEY = "note";
const PANEL_WRITER_ID = "panel-xyz";
const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("buildHostApi — host.storage reads", () => {
  it("targets the user-state route and returns the stored entry", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ value: "hi", updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    const entry = await host.storage.get("task", TASK_ID, NOTE_KEY);

    expect(entry).toEqual({ key: NOTE_KEY, value: "hi", updatedAt: TEST_UPDATED_AT });
    expect(fetchMock.mock.calls[0][0]).toContain(
      "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note",
    );
  });

  it("returns undefined on 404", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 404 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    expect(await host.storage.get("task", TASK_ID, NOTE_KEY)).toBeUndefined();
  });

  it("throws on a non-2xx, non-404 status", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 500 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    await expect(host.storage.get("task", TASK_ID, NOTE_KEY)).rejects.toThrow();
  });
});

describe("buildHostApi — host.storage writes", () => {
  it("PUTs a JSON envelope with value and a stable writer id", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(
        () => new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());

    const first = await host.storage.set("task", TASK_ID, NOTE_KEY, "a");
    await host.storage.set("task", TASK_ID, NOTE_KEY, "b");

    expect(first).toEqual({ updatedAt: TEST_UPDATED_AT });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note");
    expect(init.method).toBe("PUT");
    const firstBody = JSON.parse(init.body as string);
    const secondBody = JSON.parse(fetchMock.mock.calls[1][1].body as string);
    expect(firstBody.value).toBe("a");
    expect(firstBody.writerId).toBeTypeOf("string");
    expect(firstBody.writerId).toBe(secondBody.writerId);
  });

  it("forwards ifUnmodifiedSince and throws on conflict", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 409 }));
    global.fetch = fetchMock as unknown as typeof fetch;
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());

    await expect(
      host.storage.set("task", TASK_ID, NOTE_KEY, "hello", {
        ifUnmodifiedSince: TEST_UPDATED_AT,
      }),
    ).rejects.toThrow(/modified since/i);

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.ifUnmodifiedSince).toBe(TEST_UPDATED_AT);
  });

  it("sends DELETE with the writer id in the query", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());

    await host.storage.delete("task", TASK_ID, NOTE_KEY);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note?writerId=");
    expect(init.method).toBe("DELETE");
  });

  it("appends the surface writer id to the per-tab default", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());

    await host.storage.set("task", TASK_ID, NOTE_KEY, "hello", { writerId: PANEL_WRITER_ID });
    await host.storage.delete("task", TASK_ID, NOTE_KEY, { writerId: PANEL_WRITER_ID });

    const setBody = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(setBody.writerId).toMatch(/^.+:panel-xyz$/);
    expect(setBody.writerId).not.toBe(PANEL_WRITER_ID);
    const deleteURL = fetchMock.mock.calls[1][0];
    expect(deleteURL).toMatch(/writerId=[^&]+%3Apanel-xyz$/);
    expect(deleteURL).not.toContain(`writerId=${PANEL_WRITER_ID}`);
  });
});

describe("buildHostApi — host.storage lists and subscriptions", () => {
  it("returns entries in server order", async () => {
    const entries = [
      { key: "alpha", value: 1, updatedAt: TEST_UPDATED_AT },
      { key: "zeta", value: 2, updatedAt: "2026-01-01T00:00:01Z" },
    ];
    global.fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ entries }), { status: 200 }),
      ) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    expect(await host.storage.list("task", TASK_ID)).toEqual(entries);
  });

  it("returns an unsubscribe wired through the plugin registry", async () => {
    const { pluginRegistry } = await import("./registry");
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    const before = pluginRegistry.getWsHandlers("plugin.user-state.updated").length;

    const unsubscribe = host.storage.subscribe({}, () => {});
    expect(pluginRegistry.getWsHandlers("plugin.user-state.updated")).toHaveLength(before + 1);

    unsubscribe();
    expect(pluginRegistry.getWsHandlers("plugin.user-state.updated")).toHaveLength(before);
  });
});

describe("buildHostApi — insecure origin", () => {
  it("still creates a writer id when crypto.randomUUID is unavailable", async () => {
    vi.stubGlobal("crypto", {});
    vi.resetModules();
    const freshHostApi = await import("./host-api");
    const { createAppStore: freshCreateStore } = await import("@/lib/state/store");
    global.fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      ) as unknown as typeof fetch;

    const host = freshHostApi.buildHostApi(NOTES_PLUGIN_ID, freshCreateStore());
    await host.storage.set("task", TASK_ID, NOTE_KEY, { body: "hi" });

    const fetchMock = global.fetch as unknown as ReturnType<typeof vi.fn>;
    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.writerId).toBeTypeOf("string");
    expect(body.writerId.length).toBeGreaterThan(0);
  });
});
