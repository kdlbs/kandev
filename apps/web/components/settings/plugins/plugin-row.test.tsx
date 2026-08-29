import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { PluginRow, type PluginRowUpdateState } from "./plugin-row";
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

function marketplaceEntry(overrides: Partial<MarketplaceEntry> = {}): MarketplaceEntry {
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
    ...overrides,
  };
}

function updateState(overrides: Partial<PluginRowUpdateState> = {}): PluginRowUpdateState {
  return {
    latest: marketplaceEntry(),
    hasUpdate: true,
    checked: true,
    busy: false,
    ...overrides,
  };
}

const noop = () => undefined;
const UPDATE_BUTTON_TESTID = "plugin-update-acme";
const LATEST_VERSION_TESTID = "plugin-latest-version-acme";
const UPDATE_BADGE_TESTID = "plugin-update-available-acme";
const NOT_IN_MARKETPLACE_TESTID = "plugin-not-in-marketplace-acme";
const INLINE_UNINSTALL_CONFIRMATION_TESTID = "plugin-uninstall-inline-confirmation";

// baseProps carries the always-required callbacks/flags so each test only
// spells out the props it is actually asserting on.
const baseProps = {
  busy: false,
  autoUpdateDefault: false,
  autoUpdateBusy: false,
  onEnable: noop,
  onDisable: noop,
  onSetAutoUpdate: noop,
};

/** Test ids reused across these cases. */
const REPO_LINK_TESTID = "plugin-repo-link";
const AUTO_UPDATE_TESTID = "plugin-auto-update-acme";

/** The class that lifts a control above the card's overlay link. */
const ABOVE_OVERLAY = "z-10";

describe("PluginRow settings link", () => {
  it("covers the whole card with a named link to the plugin's settings page", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    const link = screen.getByTestId("plugin-row-link-acme");
    expect(link.getAttribute("href")).toBe("/settings/plugins/acme");
    // The link carries no text of its own, so the accessible name has to come
    // from the label — without it the card is an unnamed link.
    expect(link.getAttribute("aria-label")).toBe("Open settings for Acme");
    expect(link.className).toContain("absolute");
    expect(link.className).toContain("inset-0");
  });

  it("percent-encodes a plugin id that is not URL-safe", () => {
    render(<PluginRow {...baseProps} plugin={plugin({ id: "acme/one" })} />);
    expect(screen.getByTestId("plugin-row-link-acme/one").getAttribute("href")).toBe(
      "/settings/plugins/acme%2Fone",
    );
  });

  it("keeps the row's own controls above the overlay link", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin({ repo_url: "https://github.com/kdlbs/kandev-plugin-acme" })}
      />,
    );
    // Anything interactive must sit at z-10; under the overlay it is unclickable.
    expect(screen.getByTestId(REPO_LINK_TESTID).className).toContain(ABOVE_OVERLAY);
    expect(screen.getByRole("button", { name: "Disable" }).parentElement?.className).toContain(
      ABOVE_OVERLAY,
    );
    expect(screen.getByTestId(AUTO_UPDATE_TESTID).parentElement?.className).toContain(
      ABOVE_OVERLAY,
    );
  });
});

describe("PluginRow setup-required badge", () => {
  it("flags a plugin whose required settings are unset", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} needsSetup />);
    expect(screen.getByTestId("plugin-setup-required-acme").textContent).toBe("Setup required");
  });

  it("renders no badge when the plugin needs no setup", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId("plugin-setup-required-acme")).toBeNull();
  });
});

describe("PluginRow update button", () => {
  it("shows an Update button with the new version and fires onUpdate", () => {
    const onUpdate = vi.fn();
    render(
      <PluginRow {...baseProps} plugin={plugin()} update={updateState()} onUpdate={onUpdate} />,
    );
    const button = screen.getByTestId(UPDATE_BUTTON_TESTID);
    expect(button.textContent).toContain("Update to v2.0.0");
    fireEvent.click(button);
    expect(onUpdate).toHaveBeenCalledWith(marketplaceEntry());
  });

  it("renders no Update button when there is no pending update", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId(UPDATE_BUTTON_TESTID)).toBeNull();
  });

  it("shows a spinner and 'Updating…' while a manual update is in flight, and disables Enable/Disable/Uninstall", () => {
    render(
      <PluginRow
        {...baseProps}
        busy
        plugin={plugin()}
        update={updateState({ busy: true })}
        onUpdate={noop}
      />,
    );
    const button = screen.getByTestId(UPDATE_BUTTON_TESTID);
    expect(button.textContent).toContain("Updating");
    expect(button.querySelector(".animate-spin")).not.toBeNull();
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Uninstall" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("shows an inline error after a failed manual update, and keeps the button clickable", () => {
    const onUpdate = vi.fn();
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({ error: "bad checksum" })}
        onUpdate={onUpdate}
      />,
    );
    expect(screen.getByTestId("plugin-update-error-acme").textContent).toContain("bad checksum");
    const button = screen.getByTestId(UPDATE_BUTTON_TESTID);
    expect((button as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(button);
    expect(onUpdate).toHaveBeenCalled();
  });
});

describe("PluginRow latest version info", () => {
  it("shows the latest version without duplicating the update button with a badge", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} update={updateState()} onUpdate={noop} />);
    expect(screen.getByTestId(LATEST_VERSION_TESTID).textContent).toContain("Latest v2.0.0");
    expect(screen.queryByTestId(UPDATE_BADGE_TESTID)).toBeNull();
    expect(screen.getByTestId(UPDATE_BUTTON_TESTID).getAttribute("data-variant")).toBe("default");
  });

  it("shows the latest version with no badge and no button when already up to date", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({
          latest: marketplaceEntry({ version: "1.0.0", install_state: "installed" }),
          hasUpdate: false,
        })}
      />,
    );
    expect(screen.getByTestId(LATEST_VERSION_TESTID).textContent).toContain("Latest v1.0.0");
    expect(screen.queryByTestId(UPDATE_BADGE_TESTID)).toBeNull();
    expect(screen.queryByTestId(UPDATE_BUTTON_TESTID)).toBeNull();
  });

  it("shows a not-in-marketplace hint once checked and absent from every catalog", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({ latest: undefined, hasUpdate: false, checked: true })}
      />,
    );
    expect(screen.getByTestId(NOT_IN_MARKETPLACE_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(LATEST_VERSION_TESTID)).toBeNull();
  });

  // Regression: a plugin carried only by a source that failed this check is
  // unknown, not delisted — claiming "Not in the marketplace" reads as removal.
  it("withholds the not-in-marketplace hint when the check only reached some sources", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({
          latest: undefined,
          hasUpdate: false,
          checked: true,
          sourcesDegraded: true,
        })}
      />,
    );
    expect(screen.queryByTestId(NOT_IN_MARKETPLACE_TESTID)).toBeNull();
    expect(screen.queryByTestId(LATEST_VERSION_TESTID)).toBeNull();
  });

  it("still shows a known latest version when some other source is degraded", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({ sourcesDegraded: true })}
        onUpdate={noop}
      />,
    );
    expect(screen.getByTestId(LATEST_VERSION_TESTID).textContent).toContain("Latest v2.0.0");
    expect(screen.queryByTestId(UPDATE_BADGE_TESTID)).toBeNull();
    expect(screen.getByTestId(UPDATE_BUTTON_TESTID).getAttribute("data-variant")).toBe("default");
  });

  it("shows neither the latest version nor the not-in-marketplace hint before the first successful check", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState({ latest: undefined, hasUpdate: false, checked: false })}
      />,
    );
    expect(screen.queryByTestId(LATEST_VERSION_TESTID)).toBeNull();
    expect(screen.queryByTestId(NOT_IN_MARKETPLACE_TESTID)).toBeNull();
  });

  it("renders nothing update-related when no update prop is passed at all", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId(LATEST_VERSION_TESTID)).toBeNull();
    expect(screen.queryByTestId(NOT_IN_MARKETPLACE_TESTID)).toBeNull();
    expect(screen.queryByTestId(UPDATE_BADGE_TESTID)).toBeNull();
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
    const link = screen.getByTestId(REPO_LINK_TESTID);
    expect(link.getAttribute("href")).toBe("https://github.com/kdlbs/kandev-plugin-acme");
  });

  it("renders no Repo link when the plugin declares no repo_url", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);
    expect(screen.queryByTestId(REPO_LINK_TESTID)).toBeNull();
  });

  it("renders no Repo link for a non-http(s) repo_url scheme", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin({ repo_url: "javascript:alert(document.cookie)" })}
      />,
    );
    expect(screen.queryByTestId(REPO_LINK_TESTID)).toBeNull();
  });
});

describe("PluginRow settings link", () => {
  it("shows a labeled settings link beside the row actions", () => {
    render(<PluginRow {...baseProps} plugin={plugin()} />);

    const settingsLink = screen.getByTestId("plugin-settings-link-acme");
    expect(settingsLink.textContent).toContain("Settings");
    expect(settingsLink.getAttribute("href")).toBe("/settings/plugins/acme");
    expect(settingsLink.className).toContain("min-h-11");
  });
});

describe("PluginRow error recovery", () => {
  it("shows the failure diagnostic and an Enable action for errored plugins", () => {
    const onEnable = vi.fn();
    const p = plugin({
      status: "error",
      last_error: "plugins/runtime: handshake failed",
      last_error_at: "2026-08-02T12:34:56Z",
    });
    render(<PluginRow {...baseProps} plugin={p} onEnable={onEnable} />);

    expect(screen.getByRole("alert").textContent).toContain("plugins/runtime: handshake failed");
    const enable = screen.getByRole("button", { name: "Enable" });
    fireEvent.click(enable);
    expect(onEnable).toHaveBeenCalledWith(p);
  });
});

describe("PluginRow auto-update toggle", () => {
  it("reflects the global default when the plugin has no override", () => {
    render(<PluginRow {...baseProps} plugin={plugin({ auto_update: null })} autoUpdateDefault />);
    const toggle = screen.getByTestId(AUTO_UPDATE_TESTID);
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    // No override → no "override" badge and no Reset affordance.
    expect(screen.queryByTestId("plugin-auto-update-reset-acme")).toBeNull();
  });

  it("prefers the per-plugin override over the global default", () => {
    render(<PluginRow {...baseProps} plugin={plugin({ auto_update: false })} autoUpdateDefault />);
    const toggle = screen.getByTestId(AUTO_UPDATE_TESTID);
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
    fireEvent.click(screen.getByTestId(AUTO_UPDATE_TESTID));
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

describe("PluginRow member view", () => {
  it("keeps plugin details available without rendering administrator actions", () => {
    render(
      <PluginRow
        {...baseProps}
        plugin={plugin()}
        update={updateState()}
        onUpdate={noop}
        canManage={false}
      />,
    );

    expect(screen.getByTestId("plugin-row-link-acme")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Disable" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Uninstall" })).toBeNull();
    expect(screen.queryByTestId("plugin-update-acme")).toBeNull();
    expect(screen.queryByTestId(AUTO_UPDATE_TESTID)).toBeNull();
  });
});

describe("PluginRow uninstall confirmation", () => {
  const pluginName = "Acme Tools";

  it("closes an open confirmation when management permission is revoked", () => {
    const p = plugin({ display_name: pluginName });
    const onConfirmUninstall = vi.fn();
    const view = render(
      <PluginRow
        {...baseProps}
        plugin={p}
        isFinePointer={false}
        canManage
        onConfirmUninstall={onConfirmUninstall}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /^Uninstall$/i }));
    expect(screen.getByTestId(INLINE_UNINSTALL_CONFIRMATION_TESTID)).toBeTruthy();

    view.rerender(
      <PluginRow
        {...baseProps}
        plugin={p}
        isFinePointer={false}
        canManage={false}
        onConfirmUninstall={onConfirmUninstall}
      />,
    );
    expect(screen.queryByTestId(INLINE_UNINSTALL_CONFIRMATION_TESTID)).toBeNull();

    view.rerender(
      <PluginRow
        {...baseProps}
        plugin={p}
        isFinePointer={false}
        canManage
        onConfirmUninstall={onConfirmUninstall}
      />,
    );
    expect(screen.queryByTestId(INLINE_UNINSTALL_CONFIRMATION_TESTID)).toBeNull();
    expect(onConfirmUninstall).not.toHaveBeenCalled();
  });

  it("anchors fine-pointer confirmation to the row action and names the target", () => {
    const p = plugin({ display_name: pluginName });
    const onConfirmUninstall = vi.fn();

    render(
      <PluginRow {...baseProps} plugin={p} isFinePointer onConfirmUninstall={onConfirmUninstall} />,
    );

    fireEvent.click(screen.getByRole("button", { name: /^Uninstall$/i }));

    expect(screen.getByTestId("plugin-uninstall-confirm-popover").textContent).toContain(
      pluginName,
    );
    expect(document.querySelector('[data-slot="dialog-overlay"]')).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));
    expect(screen.queryByTestId("plugin-uninstall-confirm-popover")).toBeNull();
  });

  it("uses inline touch actions on coarse pointers and passes the row target on confirm", async () => {
    const p = plugin({ display_name: pluginName });
    const onConfirmUninstall = vi.fn();

    render(
      <PluginRow
        {...baseProps}
        plugin={p}
        isFinePointer={false}
        onConfirmUninstall={onConfirmUninstall}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /^Uninstall$/i }));

    const confirmation = screen.getByTestId(INLINE_UNINSTALL_CONFIRMATION_TESTID);
    expect(confirmation.textContent).toContain(pluginName);
    expect(screen.queryByRole("button", { name: /^Uninstall$/i })).toBeNull();
    expect(screen.getByTestId("plugin-uninstall-confirm").className).toContain("h-11");
    expect(screen.getByTestId("plugin-uninstall-confirm").className).toContain("min-w-11");

    fireEvent.click(screen.getByTestId("plugin-uninstall-confirm"));
    await waitFor(() => expect(onConfirmUninstall).toHaveBeenCalledWith(p));
  });
});
