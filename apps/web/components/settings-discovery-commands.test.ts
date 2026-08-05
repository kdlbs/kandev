import { describe, expect, it, vi } from "vitest";
import type { ResolvedSettingsDiscoveryItem } from "@/lib/settings-discovery/types";
import { buildSettingsDiscoveryCommands } from "./settings-discovery-commands";

function item(overrides: Partial<ResolvedSettingsDiscoveryItem> = {}) {
  return {
    id: "terminal-font-size",
    kind: "control" as const,
    label: "Terminal Font Size",
    aliases: ["text size"],
    breadcrumb: ["General", "Terminal"],
    groupId: "general",
    groupLabel: "General",
    href: "/settings/general/terminal#setting-terminal-font-size",
    targetId: "setting-terminal-font-size",
    order: 31,
    ...overrides,
  };
}

describe("buildSettingsDiscoveryCommands", () => {
  it("creates search-only commands with stable ids, aliases, and display-only context", () => {
    const [command] = buildSettingsDiscoveryCommands([item()], vi.fn(), "Settings");

    expect(command).toMatchObject({
      id: "setting:terminal-font-size",
      label: "Terminal Font Size",
      group: "Settings",
      context: "Settings › General › Terminal",
      searchOnly: true,
      keywords: ["text size", "General", "Terminal"],
    });
  });

  it("retains legacy command ids and aliases for existing settings pages", () => {
    const [command] = buildSettingsDiscoveryCommands(
      [item({ id: "agents", kind: "page", label: "Agents", aliases: [], breadcrumb: [] })],
      vi.fn(),
      "Settings",
      { agents: ["agent settings", "agent profiles"] },
    );

    expect(command.id).toBe("settings-agents");
    expect(command.keywords).toEqual(["agent settings", "agent profiles"]);
  });

  it("navigates to the catalog target", () => {
    window.history.replaceState({}, "", "/settings/general/appearance");
    const push = vi.fn();
    const [command] = buildSettingsDiscoveryCommands([item()], push, "Settings");

    command.action?.();

    expect(push).toHaveBeenCalledWith("/settings/general/terminal#setting-terminal-font-size");
  });
});
