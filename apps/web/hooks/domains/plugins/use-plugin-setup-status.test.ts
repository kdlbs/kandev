import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { PluginRecord } from "@/lib/types/plugins";

const getPluginConfig = vi.fn();

vi.mock("@/lib/api/domains/plugins-api", () => ({
  getPluginConfig: (...args: unknown[]) => getPluginConfig(...args),
}));

import { usePluginSetupStatus } from "./use-plugin-setup-status";

function plugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme",
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme",
    description: "",
    author: "acme",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/p",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

/** A schema with one required secret plus one optional field. */
function schema(required: string[] = ["api_token"]) {
  return {
    type: "object",
    required,
    properties: {
      api_token: { type: "string", title: "API token", secret: true },
      greeting: { type: "string", title: "Greeting" },
    },
  };
}

beforeEach(() => {
  getPluginConfig.mockReset();
  getPluginConfig.mockResolvedValue({});
});

afterEach(() => cleanup());

describe("usePluginSetupStatus", () => {
  it("flags a plugin whose required field has no stored value", async () => {
    const plugins = [plugin({ config_schema: schema() })];
    const { result } = renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(result.current.has("acme")).toBe(true));
  });

  it("does not flag a plugin whose required field is set", async () => {
    getPluginConfig.mockResolvedValue({ api_token: "********" });
    const plugins = [plugin({ config_schema: schema() })];
    const { result } = renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(getPluginConfig).toHaveBeenCalled());
    expect(result.current.has("acme")).toBe(false);
  });

  it("treats a schema default as a value, so a defaulted field needs no setup", async () => {
    const plugins = [
      plugin({
        config_schema: {
          type: "object",
          required: ["endpoint"],
          properties: { endpoint: { type: "string", default: "https://example.test" } },
        },
      }),
    ];
    const { result } = renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(getPluginConfig).toHaveBeenCalled());
    expect(result.current.size).toBe(0);
  });

  it("never probes a plugin with no required fields", async () => {
    const plugins = [
      plugin({ id: "no-schema" }),
      plugin({ id: "optional-only", config_schema: schema([]) }),
      // A required boolean is never "missing": false is a legitimate value.
      plugin({
        id: "bool-only",
        config_schema: {
          type: "object",
          required: ["verbose"],
          properties: { verbose: { type: "boolean" } },
        },
      }),
    ];
    const { result } = renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(result.current.size).toBe(0));
    expect(getPluginConfig).not.toHaveBeenCalled();
  });

  it("skips uninstalled plugins", async () => {
    const plugins = [plugin({ status: "uninstalled", config_schema: schema() })];
    renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(getPluginConfig).not.toHaveBeenCalled());
  });

  it("does not flag a plugin whose config read failed", async () => {
    getPluginConfig.mockRejectedValue(new Error("offline"));
    const plugins = [plugin({ config_schema: schema() })];
    const { result } = renderHook(() => usePluginSetupStatus(plugins));

    await waitFor(() => expect(getPluginConfig).toHaveBeenCalled());
    expect(result.current.size).toBe(0);
  });

  it("re-probes when a plugin is upgraded, not on every store update", async () => {
    const plugins = [plugin({ config_schema: schema() })];
    const { rerender } = renderHook(({ items }) => usePluginSetupStatus(items), {
      initialProps: { items: plugins },
    });
    await waitFor(() => expect(getPluginConfig).toHaveBeenCalledTimes(1));

    // Same plugins, fresh array identity (a store update elsewhere).
    rerender({ items: [plugin({ config_schema: schema() })] });
    await waitFor(() => expect(getPluginConfig).toHaveBeenCalledTimes(1));

    rerender({ items: [plugin({ version: "2.0.0", config_schema: schema() })] });
    await waitFor(() => expect(getPluginConfig).toHaveBeenCalledTimes(2));
  });
});
