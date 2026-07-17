import { describe, expect, it } from "vitest";
import { toActivePlugin } from "./active-plugin";
import type { PluginRecord } from "@/lib/types/plugins";

function record(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme-tools",
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme Tools",
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/home/user/.kandev/plugins/acme-tools/1.0.0",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ui: { bundle: "/ui/bundle.js" },
    ...overrides,
  };
}

describe("toActivePlugin", () => {
  it("builds the bundle URL from the plugin id, mirroring the backend's boot payload mapping", () => {
    const result = toActivePlugin(record());
    expect(result).not.toBeNull();
    expect(result?.bundleUrl).toBe("/api/plugins/acme-tools/bundle");
    expect(result?.id).toBe("acme-tools");
    expect(result?.name).toBe("Acme Tools");
  });

  it("maps ui.styles to /api/plugins/:id/ui/:style, mirroring pluginStyleURLs on the backend", () => {
    const result = toActivePlugin(
      record({ ui: { bundle: "/ui/bundle.js", styles: ["/ui/a.css", "/ui/b.css"] } }),
    );
    expect(result?.styleUrls).toEqual([
      "/api/plugins/acme-tools/ui/ui/a.css",
      "/api/plugins/acme-tools/ui/ui/b.css",
    ]);
  });

  it("omits styleUrls when the plugin declares no styles", () => {
    const result = toActivePlugin(record({ ui: { bundle: "/ui/bundle.js" } }));
    expect(result?.styleUrls).toBeUndefined();
  });

  it("returns null when the plugin declares no UI bundle", () => {
    const result = toActivePlugin(record({ ui: undefined }));
    expect(result).toBeNull();
  });

  it("returns null when ui.bundle is an empty string", () => {
    const result = toActivePlugin(record({ ui: { bundle: "" } }));
    expect(result).toBeNull();
  });
});
