import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { SETTINGS_DISCOVERY_DEFINITIONS, SETTINGS_DISCOVERY_ROUTE_EXCLUSIONS } from "./catalog";
import { resolveSettingsDiscovery } from "./resolve";

const GITHUB_CONNECTION_ID = "integration-github-connection";
const STABLE_CONTROL_IDS = [
  "appearance-color-theme",
  "appearance-rich-output-motion",
  "appearance-startup-page",
  "appearance-display-language",
  "terminal-preferred-shell",
  "notifications-desktop",
  "notifications-sound",
  "keyboard-submit-shortcut",
  "keyboard-command-shortcuts",
  "task-actions-agent-profile",
  "task-actions-archive-confirmation",
  "task-actions-transcript-navigation",
  "layouts-profiles",
  "sprites-connection",
  "sprites-instances",
  "utility-default-model",
  "utility-actions",
  "external-mcp-endpoints",
  "external-mcp-snippets",
] as const;

const translate = (key: string) =>
  (
    ({
      "common:agents": "Agents",
      "common:executors": "Executors",
      "common:workspaces": "Workspaces",
      "settings:general": "General",
      "sidebar:repositories": "Repositories",
      "workflows:workflows": "Workflows",
    }) as Record<string, string>
  )[key] ?? key;

describe("settings discovery catalog invariants", () => {
  it("uses unique ids, valid parents, and targets for every control", () => {
    const ids = new Set(SETTINGS_DISCOVERY_DEFINITIONS.map((entry) => entry.id));
    const targets = SETTINGS_DISCOVERY_DEFINITIONS.flatMap((entry) =>
      entry.targetId ? [entry.targetId] : [],
    );

    expect(ids.size).toBe(SETTINGS_DISCOVERY_DEFINITIONS.length);
    expect(new Set(targets).size).toBe(targets.length);
    for (const entry of SETTINGS_DISCOVERY_DEFINITIONS) {
      if (entry.parentId) {
        expect(ids.has(entry.parentId), `${entry.id} has missing parent ${entry.parentId}`).toBe(
          true,
        );
      }
      if (entry.kind === "control") {
        expect(entry.targetId, `${entry.id} has no exact target`).toMatch(/^setting-/);
      }
    }
  });

  it("indexes stable controls across general and standalone settings", () => {
    const ids = SETTINGS_DISCOVERY_DEFINITIONS.map((entry) => entry.id);

    expect(ids).toEqual(expect.arrayContaining([...STABLE_CONTROL_IDS]));
  });

  it("no longer indexes the removed core Voice Mode controls", () => {
    // Voice Mode moved to a plugin; its settings live on the plugin's own
    // page and must not reappear in the core discovery catalog.
    expect(SETTINGS_DISCOVERY_DEFINITIONS.filter((entry) => entry.id.startsWith("voice"))).toEqual(
      [],
    );
  });

  it("indexes stable system and account sections", () => {
    const ids = SETTINGS_DISCOVERY_DEFINITIONS.map((entry) => entry.id);

    expect(ids).toEqual(
      expect.arrayContaining([
        "system-database",
        "system-backups",
        "system-logs",
        "system-licenses",
        "system-storage-actions",
        "system-storage-schedule",
        "system-storage-workspaces",
        "system-storage-go-cache",
        "system-storage-docker",
        "system-storage-quarantine",
        "account-password",
        "account-sessions",
      ]),
    );
    // Integrations are workspace-scoped: never in the static catalog.
    expect(ids.some((id) => id.startsWith("integration-"))).toBe(false);
  });

  it("has no parent cycles", () => {
    const byId = new Map(SETTINGS_DISCOVERY_DEFINITIONS.map((entry) => [entry.id, entry]));

    for (const entry of SETTINGS_DISCOVERY_DEFINITIONS) {
      const visited = new Set<string>();
      let current = entry;
      while (current.parentId) {
        expect(visited.has(current.id), `cycle includes ${current.id}`).toBe(false);
        visited.add(current.id);
        const parent = byId.get(current.parentId);
        if (!parent) break;
        current = parent;
      }
    }
  });

  it("covers every static settings route or records an explicit reason", () => {
    const here = path.dirname(fileURLToPath(import.meta.url));
    const source = readFileSync(path.join(here, "../../src/settings-routes.tsx"), "utf8");
    const routes = [...source.matchAll(/^\s*"([^"]+)":\s*\(\)\s*=>/gm)].map((match) => match[1]);
    expect(routes.length).toBeGreaterThan(20);

    const covered = new Set(SETTINGS_DISCOVERY_DEFINITIONS.map((entry) => entry.href));
    const missing = routes.filter(
      (route) => !covered.has(route) && !(route in SETTINGS_DISCOVERY_ROUTE_EXCLUSIONS),
    );

    expect(missing).toEqual([]);
    for (const reason of Object.values(SETTINGS_DISCOVERY_ROUTE_EXCLUSIONS)) {
      expect(reason.length).toBeGreaterThan(12);
    }
  });
});

describe("resolveSettingsDiscovery dynamic entries", () => {
  it("uses the capitalized Settings navigation labels for workspace pages", () => {
    const resolved = resolveSettingsDiscovery({
      t: (key) => t(key),
      showAccount: false,
      showUsers: false,
      workspaces: [{ id: "workspace-1", name: "Main Workspace" }],
      agents: [],
      executors: [],
    });

    expect(
      ["repositories", "workflows"].map(
        (suffix) => resolved.find((entry) => entry.id === `workspace:workspace-1:${suffix}`)?.label,
      ),
    ).toEqual(["Repositories", "Workflows"]);
  });

  it("encodes dynamic workspace, agent profile, and executor profile routes", () => {
    const resolved = resolveSettingsDiscovery({
      t: translate,
      showAccount: true,
      showUsers: true,
      workspaces: [{ id: "workspace / one", name: "Main Workspace" }],
      agents: [
        {
          name: "Claude Code",
          profiles: [{ id: "profile / one", name: "Sonnet", agentDisplayName: "Claude" }],
        },
      ],
      executors: [
        {
          type: "docker",
          profiles: [{ id: "executor / one", name: "Docker Local" }],
        },
      ],
    });

    expect(resolved.find((entry) => entry.id === "workspace:workspace / one")?.href).toBe(
      "/settings/workspaces/workspace%20%2F%20one",
    );
    expect(resolved.find((entry) => entry.id === "agent-profile:profile / one")?.href).toBe(
      "/settings/agents/Claude%20Code/profiles/profile%20%2F%20one",
    );
    expect(resolved.find((entry) => entry.id === "executor-profile:executor / one")?.href).toBe(
      "/settings/executors/executor%20%2F%20one",
    );
    expect(resolved.find((entry) => entry.id === "workspace:workspace / one:name")?.href).toBe(
      "/settings/workspaces/workspace%20%2F%20one#setting-workspace-workspace%20%2F%20one-name",
    );
    expect(
      resolved.find((entry) => entry.id === "agent-profile:profile / one:environment-variables")
        ?.href,
    ).toBe(
      "/settings/agents/Claude%20Code/profiles/profile%20%2F%20one#setting-agent-profile-profile%20%2F%20one-environment-variables",
    );
    expect(
      resolved.find((entry) => entry.id === "executor-profile:executor / one:prepare-script")?.href,
    ).toBe(
      "/settings/executors/executor%20%2F%20one#setting-executor-profile-executor%20%2F%20one-prepare-script",
    );
  });

  it("uses safe dynamic names as labels and translated ancestors as breadcrumbs", () => {
    const resolved = resolveSettingsDiscovery({
      t: translate,
      showAccount: false,
      showUsers: false,
      workspaces: [{ id: "workspace-1", name: "Personal value" }],
      agents: [],
      executors: [],
    });
    const workspace = resolved.find((entry) => entry.id === "workspace:workspace-1");
    const repository = resolved.find((entry) => entry.id === "workspace:workspace-1:repositories");

    expect(workspace?.label).toBe("Personal value");
    expect(workspace?.breadcrumb).toEqual(["Workspaces"]);
    expect(repository?.label).toBe("Repositories");
    expect(repository?.breadcrumb).toEqual(["Workspaces", "Personal value"]);
  });
});

describe("resolveSettingsDiscovery visibility", () => {
  it("omits account and user-management entries when their rendering gates are closed", () => {
    const hidden = resolveSettingsDiscovery({
      t: translate,
      showAccount: false,
      showUsers: false,
      workspaces: [],
      agents: [],
      executors: [],
    });
    const visible = resolveSettingsDiscovery({
      t: translate,
      showAccount: true,
      showUsers: true,
      workspaces: [],
      agents: [],
      executors: [],
    });

    expect(hidden.some((entry) => entry.id === "account-security")).toBe(false);
    expect(hidden.some((entry) => entry.id === "system-users")).toBe(false);
    expect(hidden.some((entry) => entry.id === GITHUB_CONNECTION_ID)).toBe(false);
    expect(visible.some((entry) => entry.id === "account-security")).toBe(true);
    expect(visible.some((entry) => entry.id === "account-tokens")).toBe(true);
    expect(visible.some((entry) => entry.id === "system-users")).toBe(true);
  });

  it("shows integration connection sections only when a workspace can render them", () => {
    const withoutWorkspace = resolveSettingsDiscovery({
      t: translate,
      showAccount: false,
      showUsers: false,
      workspaces: [],
      agents: [],
      executors: [],
    });
    const withWorkspace = resolveSettingsDiscovery({
      t: translate,
      showAccount: false,
      showUsers: false,
      workspaces: [{ id: "workspace-1", name: "Main" }],
      agents: [],
      executors: [],
    });

    expect(
      withoutWorkspace.some((entry) => entry.id.endsWith("integration-github-connection")),
    ).toBe(false);
    const connection = withWorkspace.find((entry) =>
      entry.id.endsWith("integration-github-connection"),
    );
    expect(connection?.href).toBe(
      "/settings/workspaces/workspace-1/integrations/github#setting-integration-github-connection",
    );
    // Per-workspace results carry the workspace name as their path prefix.
    expect(connection?.breadcrumb[1]).toBe("Main");
  });
});
