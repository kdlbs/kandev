import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { PluginRow } from "./plugin-row";
import type { MarketplaceEntry, PluginRecord } from "@/lib/types/plugins";

afterEach(() => cleanup());

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

function updateEntry(): MarketplaceEntry {
  return {
    id: "acme",
    name: "Acme",
    description: "",
    author: "acme",
    categories: [],
    icon_url: "",
    repo_url: "",
    version: "2.0.0",
    min_kandev_version: "",
    package_url: "https://ex/acme-2.0.0.tar.gz",
    package_sha256: "",
    stars: 0,
    updated_at: "",
    install_state: "update_available",
    source_id: "official",
    source_name: "Kandev Official",
  };
}

const noop = () => undefined;

describe("PluginRow update button", () => {
  it("shows an Update button with the new version and fires onUpdate", () => {
    const onUpdate = vi.fn();
    render(
      <PluginRow
        plugin={plugin()}
        busy={false}
        update={updateEntry()}
        onEnable={noop}
        onDisable={noop}
        onUninstall={noop}
        onUpdate={onUpdate}
      />,
    );
    const button = screen.getByTestId("plugin-update-acme");
    expect(button.textContent).toContain("Update to v2.0.0");
    fireEvent.click(button);
    expect(onUpdate).toHaveBeenCalledWith(updateEntry());
  });

  it("renders no Update button when there is no pending update", () => {
    render(
      <PluginRow
        plugin={plugin()}
        busy={false}
        onEnable={noop}
        onDisable={noop}
        onUninstall={noop}
      />,
    );
    expect(screen.queryByTestId("plugin-update-acme")).toBeNull();
  });
});
