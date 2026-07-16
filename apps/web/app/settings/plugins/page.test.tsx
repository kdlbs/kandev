import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { PluginRecord, SyncResult } from "@/lib/types/plugins";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));

const {
  enablePluginSpy,
  disablePluginSpy,
  uninstallPluginSpy,
  installPluginFromUrlSpy,
  installPluginUploadSpy,
  listPluginsSpy,
  syncPluginsSpy,
} = vi.hoisted(() => ({
  enablePluginSpy: vi.fn(),
  disablePluginSpy: vi.fn(),
  uninstallPluginSpy: vi.fn(),
  installPluginFromUrlSpy: vi.fn(),
  installPluginUploadSpy: vi.fn(),
  listPluginsSpy: vi.fn(),
  syncPluginsSpy: vi.fn(),
}));

vi.mock("@/lib/api/domains/plugins-api", () => ({
  listPlugins: (...args: unknown[]) => {
    listPluginsSpy(...args);
    return Promise.resolve([]);
  },
  enablePlugin: (...args: [string]) => {
    enablePluginSpy(...args);
    return Promise.resolve({ enabled: true });
  },
  disablePlugin: (...args: [string]) => {
    disablePluginSpy(...args);
    return Promise.resolve({ disabled: true });
  },
  uninstallPlugin: (...args: [string]) => {
    uninstallPluginSpy(...args);
    return Promise.resolve({ deleted: true });
  },
  installPluginFromUrl: (...args: [string]) => installPluginFromUrlSpy(...args),
  installPluginUpload: (...args: [File]) => installPluginUploadSpy(...args),
  syncPlugins: (...args: unknown[]) => syncPluginsSpy(...args),
}));

const { loadPluginsSpy, unloadPluginSpy } = vi.hoisted(() => ({
  loadPluginsSpy: vi.fn(),
  unloadPluginSpy: vi.fn(),
}));

vi.mock("@/lib/plugins/host", () => ({
  loadPlugins: (...args: unknown[]) => {
    loadPluginsSpy(...args);
    return Promise.resolve();
  },
  unloadPlugin: (...args: unknown[]) => unloadPluginSpy(...args),
}));

vi.mock("@/lib/plugins/host-api", () => ({
  buildHostApi: (pluginId: string) => ({ pluginId }),
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "light" }),
}));

let featureEnabled = true;
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => featureEnabled,
}));

let storeState: Record<string, unknown> = {};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
  useAppStoreApi: () => ({
    getState: () => storeState,
    setState: vi.fn(),
    subscribe: vi.fn(() => () => {}),
  }),
}));

import PluginsSettingsPage from "./page";

const PLUGIN_ID = "acme-tools";
const NEW_PLUGIN_ID = "new-plugin";
const SYNC_BUTTON_TESTID = "plugins-sync-button";
const INSTALL_TRIGGER_TESTID = "install-plugin-trigger";
const NEW_PLUGIN_URL = "https://example.test/new-plugin-1.0.0.tar.gz";

function activePlugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme Tools",
    description: "desc",
    author: "acme",
    categories: ["productivity"],
    capabilities: {},
    status: "active",
    install_path: "/home/user/.kandev/plugins/acme-tools/1.0.0",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ui: { bundle: "/ui/bundle.js", styles: ["/ui/style.css"] },
    ...overrides,
  };
}

function installedPlugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return activePlugin({
    id: NEW_PLUGIN_ID,
    display_name: "New Plugin",
    install_path: `/home/user/.kandev/plugins/${NEW_PLUGIN_ID}/1.0.0`,
    ...overrides,
  });
}

function setStoreState(plugins: PluginRecord[]) {
  storeState = {
    plugins: { items: plugins, loading: false, loaded: true, error: null },
    setPlugins: vi.fn(),
    setPluginsLoading: vi.fn(),
    setPluginsError: vi.fn(),
    upsertPlugin: vi.fn(),
    removePlugin: vi.fn(),
  };
}

function emptySyncResult(overrides: Partial<SyncResult> = {}): SyncResult {
  return { added: [], installed: [], missing: [], errors: [], ...overrides };
}

beforeEach(() => {
  installPluginFromUrlSpy.mockResolvedValue({ plugin: installedPlugin() });
  installPluginUploadSpy.mockResolvedValue({ plugin: installedPlugin() });
  syncPluginsSpy.mockResolvedValue(emptySyncResult());
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  featureEnabled = true;
  storeState = {};
});

describe("PluginsSettingsPage", () => {
  it("renders nothing when the plugins feature flag is off", () => {
    featureEnabled = false;
    setStoreState([]);

    const { container } = render(<PluginsSettingsPage />);

    expect(container.firstChild).toBeNull();
  });

  it("shows an empty state when there are no plugins", () => {
    setStoreState([]);

    render(<PluginsSettingsPage />);

    expect(screen.getByText(/no plugins/i)).toBeTruthy();
  });

  it("lists plugins with their status, version, and an unsigned badge when unverified", () => {
    setStoreState([
      activePlugin(),
      activePlugin({ id: "unsigned-one", display_name: "Unsigned Plugin", signed: false }),
    ]);

    render(<PluginsSettingsPage />);

    expect(screen.getByText("Acme Tools")).toBeTruthy();
    expect(screen.getByText(/acme-tools.*v1\.0\.0/)).toBeTruthy();
    expect(screen.getAllByText(/active/i).length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("plugin-unsigned-badge")).toHaveLength(1);
  });

  it("disables an active plugin: calls the API, unloads the bundle, and updates the store", async () => {
    setStoreState([activePlugin()]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /disable/i }));

    await vi.waitFor(() => expect(disablePluginSpy).toHaveBeenCalledWith(PLUGIN_ID));
    expect(unloadPluginSpy).toHaveBeenCalledWith(PLUGIN_ID);
    const upsertPlugin = storeState.upsertPlugin as ReturnType<typeof vi.fn>;
    expect(upsertPlugin).toHaveBeenCalledWith(
      expect.objectContaining({ id: PLUGIN_ID, status: "disabled" }),
    );
  });

  it("enables a disabled plugin: calls the API, updates the store, and reloads the bundle", async () => {
    setStoreState([activePlugin({ status: "disabled" })]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /enable/i }));

    await vi.waitFor(() => expect(enablePluginSpy).toHaveBeenCalledWith(PLUGIN_ID));
    const upsertPlugin = storeState.upsertPlugin as ReturnType<typeof vi.fn>;
    expect(upsertPlugin).toHaveBeenCalledWith(
      expect.objectContaining({ id: PLUGIN_ID, status: "active" }),
    );
    await vi.waitFor(() =>
      expect(loadPluginsSpy).toHaveBeenCalledWith(
        [expect.objectContaining({ id: PLUGIN_ID, bundleUrl: `/api/plugins/${PLUGIN_ID}/bundle` })],
        expect.any(Function),
      ),
    );
  });

  it("uninstalls a plugin after confirmation: calls the API, unloads the bundle, and removes it from the store", async () => {
    setStoreState([activePlugin()]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /uninstall/i }));
    fireEvent.click(screen.getByRole("button", { name: /confirm uninstall/i }));

    await vi.waitFor(() => expect(uninstallPluginSpy).toHaveBeenCalledWith(PLUGIN_ID));
    expect(unloadPluginSpy).toHaveBeenCalledWith(PLUGIN_ID);
    const removePlugin = storeState.removePlugin as ReturnType<typeof vi.fn>;
    expect(removePlugin).toHaveBeenCalledWith(PLUGIN_ID);
  });

  it("does not uninstall when the confirmation dialog is cancelled", () => {
    setStoreState([activePlugin()]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /uninstall/i }));
    fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));

    expect(uninstallPluginSpy).not.toHaveBeenCalled();
  });
});

describe("PluginsSettingsPage install dialog", () => {
  it("installs a plugin from a URL: calls the API, loads the bundle, and updates the store", async () => {
    setStoreState([]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(INSTALL_TRIGGER_TESTID));
    fireEvent.change(screen.getByTestId("install-plugin-url-input"), {
      target: { value: NEW_PLUGIN_URL },
    });
    fireEvent.click(screen.getByTestId("install-plugin-url-submit"));

    await vi.waitFor(() => expect(installPluginFromUrlSpy).toHaveBeenCalledWith(NEW_PLUGIN_URL));
    const upsertPlugin = storeState.upsertPlugin as ReturnType<typeof vi.fn>;
    expect(upsertPlugin).toHaveBeenCalledWith(expect.objectContaining({ id: NEW_PLUGIN_ID }));
    await vi.waitFor(() =>
      expect(loadPluginsSpy).toHaveBeenCalledWith(
        [
          expect.objectContaining({
            id: NEW_PLUGIN_ID,
            bundleUrl: `/api/plugins/${NEW_PLUGIN_ID}/bundle`,
          }),
        ],
        expect.any(Function),
      ),
    );
  });

  it("installs a plugin by uploading a file: calls the API and updates the store", async () => {
    setStoreState([]);
    const file = new File([new Uint8Array([1, 2, 3])], "new-plugin-1.0.0.tar.gz", {
      type: "application/gzip",
    });

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(INSTALL_TRIGGER_TESTID));
    // Radix TabsTrigger switches tabs on mousedown, not click (see
    // @radix-ui/react-tabs) — fireEvent.click alone never fires it.
    fireEvent.mouseDown(screen.getByTestId("install-plugin-tab-upload"));
    fireEvent.change(screen.getByTestId("install-plugin-file-input"), {
      target: { files: [file] },
    });
    fireEvent.click(screen.getByTestId("install-plugin-upload-submit"));

    await vi.waitFor(() => expect(installPluginUploadSpy).toHaveBeenCalledWith(file));
    const upsertPlugin = storeState.upsertPlugin as ReturnType<typeof vi.fn>;
    expect(upsertPlugin).toHaveBeenCalledWith(expect.objectContaining({ id: NEW_PLUGIN_ID }));
  });

  it("shows a warning toast (not a bare success toast) when the package installed but failed to start", async () => {
    installPluginFromUrlSpy.mockResolvedValueOnce({
      plugin: installedPlugin({ status: "error" }),
      warning: "plugin installed but failed to start: handshake timed out",
    });
    setStoreState([]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(INSTALL_TRIGGER_TESTID));
    fireEvent.change(screen.getByTestId("install-plugin-url-input"), {
      target: { value: NEW_PLUGIN_URL },
    });
    fireEvent.click(screen.getByTestId("install-plugin-url-submit"));

    await vi.waitFor(() =>
      expect(toast.warning).toHaveBeenCalledWith(
        "plugin installed but failed to start: handshake timed out",
      ),
    );
    expect(toast.success).not.toHaveBeenCalled();
    const upsertPlugin = storeState.upsertPlugin as ReturnType<typeof vi.fn>;
    expect(upsertPlugin).toHaveBeenCalledWith(
      expect.objectContaining({ id: NEW_PLUGIN_ID, status: "error" }),
    );
  });

  it("shows the backend's error message inline when install fails", async () => {
    installPluginFromUrlSpy.mockRejectedValueOnce(
      new Error("bad checksum for server/plugin-linux-amd64"),
    );
    setStoreState([]);

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(INSTALL_TRIGGER_TESTID));
    fireEvent.change(screen.getByTestId("install-plugin-url-input"), {
      target: { value: "https://example.test/bad.tar.gz" },
    });
    fireEvent.click(screen.getByTestId("install-plugin-url-submit"));

    await vi.waitFor(() =>
      expect(screen.getByTestId("install-plugin-error").textContent).toMatch(/bad checksum/),
    );
  });
});

describe("PluginsSettingsPage sync button", () => {
  it("syncs plugins: calls the API, refreshes the list, and toasts a summary", async () => {
    setStoreState([]);
    syncPluginsSpy.mockResolvedValueOnce(emptySyncResult({ added: ["sideloaded-plugin"] }));
    const callsBeforeClick = listPluginsSpy.mock.calls.length;

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(SYNC_BUTTON_TESTID));

    await vi.waitFor(() => expect(syncPluginsSpy).toHaveBeenCalled());
    await vi.waitFor(() =>
      expect(listPluginsSpy.mock.calls.length).toBeGreaterThan(callsBeforeClick),
    );
    const setPlugins = storeState.setPlugins as ReturnType<typeof vi.fn>;
    await vi.waitFor(() => expect(setPlugins).toHaveBeenCalledWith([]));
    await vi.waitFor(() => expect(toast.success).toHaveBeenCalledWith("Sync: 1 sideloaded"));
  });

  it("toasts 'Everything up to date' when the sync finds nothing to do", async () => {
    setStoreState([]);
    syncPluginsSpy.mockResolvedValueOnce(emptySyncResult());

    render(<PluginsSettingsPage />);
    fireEvent.click(screen.getByTestId(SYNC_BUTTON_TESTID));

    await vi.waitFor(() => expect(toast.success).toHaveBeenCalledWith("Everything up to date"));
  });

  it("shows sync errors inline", async () => {
    setStoreState([]);
    syncPluginsSpy.mockResolvedValueOnce(
      emptySyncResult({
        errors: [{ path: "/plugins/junk.tar.gz", reason: "invalid gzip stream" }],
      }),
    );

    render(<PluginsSettingsPage />);
    expect(screen.queryByTestId("plugins-sync-errors")).toBeNull();
    fireEvent.click(screen.getByTestId(SYNC_BUTTON_TESTID));

    await vi.waitFor(() =>
      expect(screen.getByTestId("plugins-sync-errors").textContent).toMatch(/invalid gzip stream/),
    );
  });

  it("toasts an error and does not refresh the list when the sync call fails", async () => {
    setStoreState([]);
    syncPluginsSpy.mockRejectedValueOnce(new Error("backend unreachable"));

    render(<PluginsSettingsPage />);
    // Let the mount-triggered usePlugins() load settle before capturing the
    // baseline call count, so it only reflects the click below.
    await vi.waitFor(() => expect(listPluginsSpy).toHaveBeenCalled());
    const callsBeforeClick = listPluginsSpy.mock.calls.length;

    fireEvent.click(screen.getByTestId(SYNC_BUTTON_TESTID));

    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith("backend unreachable"));
    expect(listPluginsSpy.mock.calls.length).toBe(callsBeforeClick);
  });
});
