import { describe, expect, it } from "vitest";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import { buildIntegrationShortcutEntries } from "./integration-shortcuts";
import type { PluginNavRegistration } from "@/lib/plugins/registry";

describe("integration navigation shortcuts", () => {
  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.1, .4
  it("covers every built-in integration with its existing page and no default chord", () => {
    const entries = buildIntegrationShortcutEntries();
    expect(entries.map(({ id, href, default: shortcut }) => [id, href, shortcut])).toEqual([
      ["integration:azure-devops", "/azure-devops", { key: "" }],
      ["integration:github", "/github", { key: "" }],
      ["integration:gitlab", "/gitlab", { key: "" }],
      ["integration:jira", "/jira", { key: "" }],
      ["integration:linear", "/linear", { key: "" }],
      ["integration:sentry", "/settings/integrations/sentry", { key: "" }],
    ]);
    expect(entries).toHaveLength(WORKSPACE_INTEGRATIONS.length);
  });

  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.2, .8
  it("includes only integration nav links and isolates IDs across plugin owners", () => {
    const item = {
      id: "review:page",
      label: "Reviews",
      path: "/plugins/reviews",
      section: "integrations",
    };
    const items = [
      { ...item, pluginId: "a:b" },
      { ...item, pluginId: "a" },
      { ...item, pluginId: "a", id: "settings", section: "settings" },
      { ...item, pluginId: "a", id: "main", section: "main" },
    ] as PluginNavRegistration[];
    const entries = buildIntegrationShortcutEntries(
      items,
      [],
      (_key, { integration }) => `Open ${integration}`,
    );
    expect(entries.slice(WORKSPACE_INTEGRATIONS.length)).toMatchObject([
      {
        id: "integration:plugin:a%3Ab:review%3Apage",
        label: "Open a:b: Reviews",
        href: "/plugins/reviews",
      },
      {
        id: "integration:plugin:a:review%3Apage",
        label: "Open a: Reviews",
        href: "/plugins/reviews",
      },
    ]);
    expect(entries).toHaveLength(WORKSPACE_INTEGRATIONS.length + 2);
    expect(entries[1].label).toBe("Open GitHub");
  });
});
