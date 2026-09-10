import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  isAdmin: false,
  plugin: {
    id: "task-manager",
    version: "1.0.0",
    display_name: "Task Manager",
    description: "",
    status: "active",
    signed: true,
  },
}));

vi.mock("@/hooks/domains/auth/use-is-admin", () => ({
  useIsAdmin: () => state.isAdmin,
}));

vi.mock("@/hooks/domains/plugins/use-plugins", () => ({
  usePlugins: () => ({ items: [state.plugin], loaded: true }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true }),
}));

vi.mock("@/components/plugins/plugin-slot", () => ({
  PluginSlot: () => <div data-testid="owner-scoped-plugin-settings" />,
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: vi.fn(),
}));

vi.mock("./use-plugin-actions", () => ({
  usePluginActions: () => ({
    busyId: null,
    uninstallBusy: false,
    confirmUninstall: vi.fn(),
  }),
}));

vi.mock("./use-plugin-config-form", () => ({
  usePluginConfigForm: () => ({
    fields: [],
    values: {},
    initialValues: {},
    configLoading: false,
    configError: null,
    saveStatus: "idle",
    isDirty: false,
    canSave: false,
    invalidReason: undefined,
    revision: "",
    handleChange: vi.fn(),
    handleSave: vi.fn(),
    discard: vi.fn(),
  }),
}));

vi.mock("./plugin-error-diagnostic", () => ({ PluginErrorDiagnostic: () => null }));
vi.mock("./plugin-manifest-card", () => ({ PluginManifestCard: () => null }));
vi.mock("./plugin-repo-link", () => ({ PluginRepoLink: () => null }));
vi.mock("./plugin-shortcuts-card", () => ({ PluginShortcutsCard: () => null }));
vi.mock("./uninstall-plugin-dialog", () => ({ PluginUninstallConfirmation: () => null }));

import { PluginDetail } from "./plugin-detail";

afterEach(() => {
  cleanup();
  state.isAdmin = false;
});

describe("PluginDetail personal settings visibility", () => {
  it("renders the owner-scoped settings slot for a non-admin without operator controls", () => {
    render(<PluginDetail pluginId={state.plugin.id} />);

    expect(screen.getByTestId("owner-scoped-plugin-settings")).not.toBeNull();
    expect(screen.queryByTestId("plugin-settings-card")).toBeNull();
  });
});
