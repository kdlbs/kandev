import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const MAIN_WORKSPACE_ID = "ws-1";
const MAIN_WORKSPACE_NAME = "Main Workspace";
const VOICE_MODE_LABEL = "Voice Mode";

const state = {
  workspaces: {
    activeId: MAIN_WORKSPACE_ID,
    items: [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }],
  },
  settingsAgents: {
    items: [
      {
        name: "claude-code",
        profiles: [{ id: "profile-1", name: "Default", agentDisplayName: "Claude Code" }],
      },
    ],
  },
  executors: {
    items: [{ id: "exec-1", type: "local", profiles: [{ id: "exec-profile-1", name: "Local" }] }],
  },
  settingsData: {
    executorsLoaded: true,
    agentsLoaded: true,
  },
  userSettings: {
    loaded: true,
    savedLayouts: [] as Array<{ id: string }>,
  },
  auth: {
    mode: "disabled" as const,
    authenticated: true,
    user: null,
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => false,
}));

vi.mock("@/hooks/domains/settings/use-secrets", () => ({
  useSecrets: () => ({ items: [{ id: "secret-1" }, { id: "secret-2" }], loaded: true, loading: false }),
}));

vi.mock("@/hooks/domains/plugins/use-plugins", () => ({
  usePlugins: () => ({ items: [{ id: "plugin-1" }], loaded: true, loading: false, error: null }),
}));

import { SettingsTree } from "./settings-tree";

describe("SettingsTree static menu", () => {
  beforeEach(() => {
    state.workspaces.activeId = MAIN_WORKSPACE_ID;
    state.workspaces.items = [{ id: MAIN_WORKSPACE_ID, name: MAIN_WORKSPACE_NAME }];
  });

  afterEach(() => cleanup());

  it("renders section headers as static text, not links or buttons", () => {
    render(<SettingsTree pathname="/settings" />);

    for (const header of ["Preferences", "Workspaces & Access", "System"]) {
      expect(screen.getByText(header)).toBeTruthy();
      expect(screen.queryByRole("link", { name: header })).toBeNull();
      expect(screen.queryByRole("button", { name: header })).toBeNull();
    }
  });

  it("holds no rows for user-created data, so menu length is constant", () => {
    render(<SettingsTree pathname="/settings" />);

    // Store has a workspace, an agent profile and an executor profile — none
    // of them may appear as menu rows.
    expect(screen.queryByRole("link", { name: MAIN_WORKSPACE_NAME })).toBeNull();
    expect(screen.queryByRole("link", { name: /Claude Code/ })).toBeNull();
    expect(screen.queryByRole("link", { name: "Local" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Repositories" })).toBeNull();

    // One row per page.
    expect(screen.getByRole("link", { name: /^Workspaces/ }).getAttribute("href")).toBe(
      "/settings/workspaces",
    );
    expect(screen.getByRole("link", { name: /^Agents/ }).getAttribute("href")).toBe(
      "/settings/agents",
    );
    expect(screen.getByRole("link", { name: /^Executors/ }).getAttribute("href")).toBe(
      "/settings/executors",
    );
  });

  it("marks the owning page row active for detail routes", () => {
    render(<SettingsTree pathname="/settings/agents/claude-code/profiles/profile-1" />);

    expect(
      screen.getByRole("link", { name: /^Agents/ }).getAttribute("data-active"),
    ).toBe("true");
    expect(
      screen.getByRole("link", { name: /^Executors/ }).getAttribute("data-active"),
    ).toBeNull();
  });

  it("hides auth-gated rows and the Access Control section when auth is disabled", () => {
    render(<SettingsTree pathname="/settings" />);

    expect(screen.queryByText("Access Control")).toBeNull();
    expect(screen.queryByRole("link", { name: "Users" })).toBeNull();
    expect(screen.queryByRole("link", { name: "API Tokens" })).toBeNull();
  });

  it("keeps Voice Mode as a page row under Agents", () => {
    render(<SettingsTree pathname="/settings/voice-mode" />);

    const link = screen.getByRole("link", { name: VOICE_MODE_LABEL });
    expect(link.getAttribute("href")).toBe("/settings/voice-mode");
    expect(link.getAttribute("data-active")).toBe("true");
  });

  it("shows item counts on rows whose page owns a list", () => {
    render(<SettingsTree pathname="/settings" />);

    // One workspace, one agent profile, one executor profile, two secrets,
    // one plugin (from the store/hook mocks above).
    expect(screen.getByRole("link", { name: "Workspaces 1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Agents 1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Executors 1" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Global Secrets 2" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Plugins 1" })).toBeTruthy();
    // Rows without a list never carry a badge.
    expect(screen.getByRole("link", { name: "Appearance" }).textContent).toBe("Appearance");
  });
});

describe("SettingsTree search", () => {
  afterEach(cleanup);

  it("preserves the normal menu until a query filters it to grouped hits", () => {
    render(<SettingsTree pathname="/settings" />);

    const search = screen.getByRole("searchbox", { name: "Search settings" });
    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL })).toBeTruthy();

    fireEvent.change(search, { target: { value: "font size" } });

    const result = screen.getByRole("link", { name: /Terminal Font Size/ });
    expect(result.getAttribute("href")).toBe(
      "/settings/preferences/terminal-editors#setting-terminal-font-size",
    );
    expect(result.textContent).toContain("Terminal & Editors");
    expect(screen.queryByRole("link", { name: VOICE_MODE_LABEL })).toBeNull();
  });

  it("prefixes per-workspace results with the workspace name", () => {
    render(<SettingsTree pathname="/settings" />);

    fireEvent.change(screen.getByRole("searchbox", { name: "Search settings" }), {
      target: { value: "Jira" },
    });

    const result = screen.getByRole("link", { name: /Jira/ });
    expect(result.getAttribute("href")).toContain(
      `/settings/workspaces/${MAIN_WORKSPACE_ID}/integrations/jira`,
    );
    expect(result.textContent).toContain(`${MAIN_WORKSPACE_NAME} › Integrations`);
  });

  it("clears a query with Escape and restores the normal menu", () => {
    render(<SettingsTree pathname="/settings" />);
    const search = screen.getByRole("searchbox", { name: "Search settings" });

    fireEvent.change(search, { target: { value: "font size" } });
    fireEvent.keyDown(search, { key: "Escape" });

    expect((search as HTMLInputElement).value).toBe("");
    expect(screen.getByRole("link", { name: VOICE_MODE_LABEL })).toBeTruthy();
  });

  it("announces an empty result without rendering the normal menu", () => {
    render(<SettingsTree pathname="/settings" />);

    fireEvent.change(screen.getByRole("searchbox", { name: "Search settings" }), {
      target: { value: "definitely missing" },
    });

    expect(screen.getByText("No matching settings")).toBeTruthy();
    expect(screen.queryByRole("link", { name: VOICE_MODE_LABEL })).toBeNull();
  });
});
