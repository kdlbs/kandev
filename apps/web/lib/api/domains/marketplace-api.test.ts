import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  addMarketplaceSource,
  deleteMarketplaceSource,
  getMarketplaceCatalog,
  listMarketplaceSources,
  refreshMarketplace,
  updateMarketplaceSource,
} from "./marketplace-api";
import { ApiError } from "../client";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function entry(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-stats",
    name: "Agent Stats",
    description: "",
    author: "kandev",
    categories: ["analytics"],
    repo_url: "https://github.com/kdlbs/kandev-plugin-agent-stats",
    version: "1.0.0",
    min_kandev_version: "",
    package_url: "https://ex/agent-stats-1.0.0.tar.gz",
    package_sha256: "",
    stars: 42,
    updated_at: "2026-07-17T00:00:00Z",
    install_state: "available",
    source_id: "official",
    source_name: "Kandev Official",
    ...overrides,
  };
}

describe("getMarketplaceCatalog", () => {
  it("GETs the catalog and encodes the query params", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ plugins: [entry()], sources: [] }));

    const result = await getMarketplaceCatalog({ q: "stats", category: "analytics", sort: "name" });

    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(
      "http://api.test/api/plugins/marketplace?q=stats&category=analytics&sort=name",
    );
    expect(result.plugins).toHaveLength(1);
    expect(result.plugins[0].id).toBe("agent-stats");
  });

  it("omits empty query params and defaults missing arrays", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));

    const result = await getMarketplaceCatalog();

    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace");
    expect(result).toEqual({ plugins: [], sources: [] });
  });
});

describe("marketplace sources", () => {
  it("lists sources, unwrapping the sources array", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        sources: [
          { id: "official", name: "Kandev Official", url: "u", enabled: true, builtin: true },
        ],
      }),
    );

    const result = await listMarketplaceSources();

    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace/sources");
    expect(result[0].builtin).toBe(true);
  });

  it("POSTs a new source with name+url as JSON", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        { id: "s1", name: "Acme", url: "https://acme/index.json", enabled: true, builtin: false },
        201,
      ),
    );

    const result = await addMarketplaceSource("Acme", "https://acme/index.json");

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace/sources");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      name: "Acme",
      url: "https://acme/index.json",
    });
    expect(result.id).toBe("s1");
  });

  it("propagates a 400 (bad/duplicate url) as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "source url must be an http(s) URL" }, 400),
    );

    await expect(addMarketplaceSource("x", "ftp://nope")).rejects.toBeInstanceOf(ApiError);
  });

  it("PATCHes a source with the partial patch body", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ id: "s1", name: "Acme", url: "u", enabled: false, builtin: false }),
    );

    await updateMarketplaceSource("s1", { enabled: false });

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace/sources/s1");
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({ enabled: false });
  });

  it("DELETEs a source and propagates a 409 for the built-in source", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ deleted: true }));
    await deleteMarketplaceSource("s1");
    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace/sources/s1");
    expect(init?.method).toBe("DELETE");

    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "the built-in source cannot be deleted" }, 409),
    );
    await expect(deleteMarketplaceSource("official")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("refreshMarketplace", () => {
  it("POSTs to the refresh endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ refreshed: true }));

    const result = await refreshMarketplace();

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/marketplace/refresh");
    expect(init?.method).toBe("POST");
    expect(result.refreshed).toBe(true);
  });
});

describe("operator boot-token on state-changing marketplace calls", () => {
  const BOOT_TOKEN = "boot-token-mkt";

  beforeEach(() => {
    (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__ = {
      runtime: { csrfToken: BOOT_TOKEN },
    };
  });

  afterEach(() => {
    delete (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__;
  });

  function tokenHeaderOf(init: FetchInit): string | undefined {
    return (init?.headers as Record<string, string> | undefined)?.["X-Kandev-Boot-Token"];
  }

  it("attaches the token to source add/update/delete and refresh", async () => {
    const calls: Array<() => Promise<unknown>> = [
      () => addMarketplaceSource("Acme", "https://acme-csrf/index.json"),
      () => updateMarketplaceSource("s1", { enabled: false }),
      () => deleteMarketplaceSource("s1"),
      () => refreshMarketplace(),
    ];
    for (const call of calls) {
      fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }));
      await call();
      const [, init] = fetchSpy.mock.calls.at(-1) ?? [];
      expect(tokenHeaderOf(init)).toBe(BOOT_TOKEN);
    }
  });

  it("does not attach the token to read-only catalog/source GETs", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ plugins: [], sources: [] }));
    await getMarketplaceCatalog();
    const [, catalogInit] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(tokenHeaderOf(catalogInit)).toBeUndefined();

    fetchSpy.mockResolvedValueOnce(jsonResponse({ sources: [] }));
    await listMarketplaceSources();
    const [, listInit] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(tokenHeaderOf(listInit)).toBeUndefined();
  });
});
