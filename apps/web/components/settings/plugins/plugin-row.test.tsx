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

// baseProps carries the always-required callbacks/flags so each test only
// spells out the props it is actually asserting on.
const baseProps = {
  busy: false,
  autoUpdateDefault: false,
  autoUpdateBusy: false,
  onEnable: noop,
  onDisable: noop,
  onUninstall: noop,
  onSetAutoUpdate: noop,
};

describe("PluginRow update button", () => {
  it("shows an Update button with the new version and fires onUpdate", () => {
    const onUpdate = vi.fn();
    render(
      <PluginRow {...baseProps} plugin={plugin()} update={updateEntry()} onUpdate={onUpdate} />,
    );
    const button = screen.getByTestId("plugin-update-acme");
    expect(button.textContent).toContain("Update to v2.0.0");
    fireEvent.click(button);
    expect(onUpdate).toHaveBeenCalledWith(updateEntry());
  });

  it("renders no Update button when there is no pending update", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId("plugin-update-acme")).toBeNull();
  });
});

describe("PluginRow repo link", () => {
  it("renders a Repo link when the plugin declares an http(s) repo_url", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin({ repo_url: "https://github.com/kdlbs/kandev-plugin-acme" })}
      />,
    );
    const link = screen.getByTestId("plugin-repo-link");
    expect(link.getAttribute("href")).toBe("https://github.com/kdlbs/kandev-plugin-acme");
  });

  it("renders no Repo link when the plugin declares no repo_url", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId("plugin-repo-link")).toBeNull();
  });

  it("renders no Repo link for a non-http(s) repo_url scheme", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin({ repo_url: "javascript:alert(document.cookie)" })}
      />,
    );
    expect(screen.queryByTestId("plugin-repo-link")).toBeNull();
  });
});

describe("PluginRow auto-update toggle", () => {
  it("reflects the global default when the plugin has no override", () => {
    render(<PluginRow {...baseProps} plugin={plugin({ auto_update: null })} autoUpdateDefault />);
    const toggle = screen.getByTestId("plugin-auto-update-acme");
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    // No override → no "override" badge and no Reset affordance.
    expect(screen.queryByTestId("plugin-auto-update-reset-acme")).toBeNull();
  });

  it("prefers the per-plugin override over the global default", () => {
    render(<PluginRow {...baseProps} plugin={plugin({ auto_update: false })} autoUpdateDefault />);
    const toggle = screen.getByTestId("plugin-auto-update-acme");
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(screen.getByTestId("plugin-auto-update-reset-acme")).toBeTruthy();
  });

  it("sets an explicit override when toggled", () => {
    const onSetAutoUpdate = vi.fn();
    const p = plugin({ auto_update: null });
    render(
      <PluginRow
        {...baseProps}
        plugin={p}
        autoUpdateDefault={false}
        onSetAutoUpdate={onSetAutoUpdate}
      />,
    );
    fireEvent.click(screen.getByTestId("plugin-auto-update-acme"));
    expect(onSetAutoUpdate).toHaveBeenCalledWith(p, true);
  });

  it("clears the override via Reset", () => {
    const onSetAutoUpdate = vi.fn();
    const p = plugin({ auto_update: true });
    render(<PluginRow {...baseProps} plugin={p} onSetAutoUpdate={onSetAutoUpdate} />);
    fireEvent.click(screen.getByTestId("plugin-auto-update-reset-acme"));
    expect(onSetAutoUpdate).toHaveBeenCalledWith(p, null);
  });
});
