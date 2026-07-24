import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  disablePlugin,
  enablePlugin,
  getPlugin,
  getPluginConfig,
  installPluginFromUrl,
  installPluginUpload,
  listPlugins,
  syncPlugins,
  uninstallPlugin,
  updatePluginConfig,
} from "./plugins-api";
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

const PLUGIN_ID = "acme-tools";
const PLUGIN_URL = "http://api.test/api/plugins/acme-tools";
const PARTIAL_INSTALL_WARNING = "plugin installed but failed to start: handshake timed out";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function plugin(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme Tools",
    description: "desc",
    author: "acme",
    categories: ["productivity"],
    capabilities: {},
    status: "registered",
    install_path: "/home/user/.kandev/plugins/acme-tools/1.0.0",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

describe("listPlugins", () => {
  it("fetches GET /api/plugins and unwraps the plugins array", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ plugins: [plugin()] }));

    const result = await listPlugins();

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins");
    expect(init?.method ?? "GET").toBe("GET");
    expect(result).toEqual([plugin()]);
  });

  it("returns an empty array when the backend omits plugins", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));

    const result = await listPlugins();

    expect(result).toEqual([]);
  });
});

describe("getPlugin", () => {
  it("fetches GET /api/plugins/:id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(plugin()));

    const result = await getPlugin(PLUGIN_ID);

    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(PLUGIN_URL);
    expect(result).toEqual(plugin());
  });

  it("propagates a 404 as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "plugin not found" }, 404));

    await expect(getPlugin("missing")).rejects.toBeInstanceOf(ApiError);
  });
});

describe("installPluginFromUrl", () => {
  it("POSTs {url} as JSON to /api/plugins/install and unwraps the plugin record", async () => {
    // Backend wraps the record as {"plugin": {...}} (internal/plugins/dto.go
    // InstallResponse) — not the bare record.
    fetchSpy.mockResolvedValueOnce(jsonResponse({ plugin: plugin({ status: "active" }) }, 201));

    const result = await installPluginFromUrl("https://example.test/acme-tools-1.0.0.tar.gz");

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/install");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      url: "https://example.test/acme-tools-1.0.0.tar.gz",
    });
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
    expect(result.plugin.id).toBe(PLUGIN_ID);
    expect(result.plugin.status).toBe("active");
    expect(result.warning).toBeUndefined();
  });

  it("surfaces a partial-install warning alongside the stored plugin record", async () => {
    // Package installed but its initial spawn/handshake failed — backend
    // leaves Plugin.Status as "error" and adds a "warning" alongside "plugin".
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          plugin: plugin({ status: "error" }),
          warning: PARTIAL_INSTALL_WARNING,
        },
        201,
      ),
    );

    const result = await installPluginFromUrl("https://example.test/acme-tools-1.0.0.tar.gz");

    expect(result.plugin.status).toBe("error");
    expect(result.warning).toBe(PARTIAL_INSTALL_WARNING);
  });

  it("propagates a 400 invalid-package error as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "bad checksum for server/plugin-linux-amd64" }, 400),
    );

    await expect(installPluginFromUrl("https://example.test/bad.tar.gz")).rejects.toMatchObject({
      status: 400,
      message: "bad checksum for server/plugin-linux-amd64",
    });
  });

  it("propagates a 409 duplicate-version error as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "version 1.0.0 already installed" }, 409));

    await expect(installPluginFromUrl("https://example.test/dup.tar.gz")).rejects.toMatchObject({
      status: 409,
      message: "version 1.0.0 already installed",
    });
  });
});

describe("installPluginUpload", () => {
  it("POSTs the file as multipart/form-data under the 'package' field and unwraps the plugin record", async () => {
    let captured: { method?: string; isFormData: boolean; hasPackageField: boolean } = {
      isFormData: false,
      hasPackageField: false,
    };
    fetchSpy.mockImplementationOnce(async (_url, init) => {
      const body = init?.body;
      captured = {
        method: init?.method,
        isFormData: body instanceof FormData,
        hasPackageField: body instanceof FormData && body.get("package") !== null,
      };
      // Backend wraps the record as {"plugin": {...}} — not the bare record.
      return jsonResponse({ plugin: plugin({ status: "active" }) }, 201);
    });
    const file = new File([new Uint8Array([1, 2, 3])], "acme-tools-1.0.0.tar.gz", {
      type: "application/gzip",
    });

    const result = await installPluginUpload(file);

    expect(captured.method).toBe("POST");
    expect(captured.isFormData).toBe(true);
    expect(captured.hasPackageField).toBe(true);
    expect(result.plugin.id).toBe(PLUGIN_ID);
    expect(result.plugin.status).toBe("active");
    expect(result.warning).toBeUndefined();
  });

  it("surfaces a partial-install warning alongside the stored plugin record", async () => {
    fetchSpy.mockImplementationOnce(async () =>
      jsonResponse(
        {
          plugin: plugin({ status: "error" }),
          warning: PARTIAL_INSTALL_WARNING,
        },
        201,
      ),
    );
    const file = new File([new Uint8Array([1, 2, 3])], "acme-tools-1.0.0.tar.gz", {
      type: "application/gzip",
    });

    const result = await installPluginUpload(file);

    expect(result.plugin.status).toBe("error");
    expect(result.warning).toBe(PARTIAL_INSTALL_WARNING);
  });

  it("does not set a Content-Type header, so the browser sets the multipart boundary", async () => {
    let headers: HeadersInit | undefined;
    fetchSpy.mockImplementationOnce(async (_url, init) => {
      headers = init?.headers;
      return jsonResponse({ plugin: plugin() }, 201);
    });
    const file = new File([new Uint8Array([1])], "plugin.tar.gz");

    await installPluginUpload(file);

    expect(headers).toBeUndefined();
  });

  it("propagates a 400 unsupported-platform error as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "no executable for platform linux-arm64" }, 400),
    );
    const file = new File([new Uint8Array([1])], "plugin.tar.gz");

    await expect(installPluginUpload(file)).rejects.toMatchObject({
      status: 400,
      message: "no executable for platform linux-arm64",
    });
  });

  it("propagates a 409 duplicate-version error as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "version 1.0.0 already installed" }, 409));
    const file = new File([new Uint8Array([1])], "plugin.tar.gz");

    await expect(installPluginUpload(file)).rejects.toMatchObject({ status: 409 });
  });
});

describe("updatePluginConfig", () => {
  it("PATCHes /api/plugins/:id with a config body", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ updated: true }));

    const result = await updatePluginConfig(PLUGIN_ID, { apiKey: "x" });

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(PLUGIN_URL);
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({ config: { apiKey: "x" } });
    expect(result).toEqual({ updated: true });
  });
});

describe("enablePlugin / disablePlugin / uninstallPlugin", () => {
  it("POSTs /api/plugins/:id/enable", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ enabled: true }));

    await enablePlugin(PLUGIN_ID);

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(`${PLUGIN_URL}/enable`);
    expect(init?.method).toBe("POST");
  });

  it("POSTs /api/plugins/:id/disable", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ disabled: true }));

    await disablePlugin(PLUGIN_ID);

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(`${PLUGIN_URL}/disable`);
    expect(init?.method).toBe("POST");
  });

  it("DELETEs /api/plugins/:id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ deleted: true }));

    await uninstallPlugin(PLUGIN_ID);

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(PLUGIN_URL);
    expect(init?.method).toBe("DELETE");
  });
});

describe("syncPlugins", () => {
  it("POSTs /api/plugins/sync and returns the SyncResult", async () => {
    const body = {
      added: ["kandev-plugin-side"],
      installed: [],
      missing: ["kandev-plugin-gone"],
      errors: [{ path: "/plugins/junk.tar.gz", reason: "invalid gzip stream" }],
    };
    fetchSpy.mockResolvedValueOnce(jsonResponse(body));

    const result = await syncPlugins();

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/plugins/sync");
    expect(init?.method).toBe("POST");
    expect(result).toEqual(body);
  });

  it("propagates a backend error as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "sync failed" }, 500));

    await expect(syncPlugins()).rejects.toMatchObject({ status: 500, message: "sync failed" });
  });
});

describe("getPluginConfig", () => {
  it("fetches GET /api/plugins/:id/config and unwraps the config object", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ config: { default_channel: "#dev" } }));

    const result = await getPluginConfig(PLUGIN_ID);

    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe(`${PLUGIN_URL}/config`);
    expect(result).toEqual({ default_channel: "#dev" });
  });

  it("returns an empty object when the backend omits config", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));
    expect(await getPluginConfig(PLUGIN_ID)).toEqual({});
  });

  it("URL-encodes the plugin id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ config: {} }));
    await getPluginConfig("my plugin");
    const [url] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toContain("my%20plugin");
  });
});
