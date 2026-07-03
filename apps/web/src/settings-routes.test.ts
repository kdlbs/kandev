import { beforeEach, describe, expect, it } from "vitest";

import { workspaceId, workflowId } from "@/lib/types/ids";
import type { ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";
import { buildSettingsInitialStateForRoute } from "./settings-routes";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const OWNER_ID = "owner-1";
const TIMESTAMP = "2026-01-01T00:00:00Z";
const SETTINGS_ROUTE = "/settings/integrations";

describe("buildSettingsInitialStateForRoute", () => {
  beforeEach(() => {
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=; path=/; max-age=0`;
  });

  describe("workspace selection", () => {
    it("prefers the workspace matching the URL path param", () => {
      const state = buildState({
        pathname: "/settings/workspace/ws-2/repositories",
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("keeps the active workspace cookie on global settings pages", () => {
      document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=ws-2; path=/`;

      const state = buildState({
        pathname: SETTINGS_ROUTE,
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("falls back to user settings when cookie has an office workspace", () => {
      document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=ws-office; path=/`;

      const state = buildState({
        pathname: SETTINGS_ROUTE,
        workspaces: [
          buildWorkspace({ id: "ws-office", office_workflow_id: workflowId("office") }),
          buildWorkspace({ id: "ws-kanban", office_workflow_id: null }),
        ],
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-kanban") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-kanban");
      expect(state.userSettings?.workspaceId).toBe("ws-kanban");
    });
  });

  describe("fallbacks", () => {
    it("falls back to the settings workspace_id when no URL param matches", () => {
      const state = buildState({
        pathname: "/settings/workspace/missing/repositories",
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-2") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-2");
      expect(state.userSettings?.workspaceId).toBe("ws-2");
    });

    it("falls back to the first workspace when neither URL param nor settings match", () => {
      const state = buildState({
        pathname: "/settings/utility-agents",
        workspaces: workspaceRows(["ws-1", "ws-2"]),
        userSettingsResponse: userSettings({ workspace_id: workspaceId("missing") }),
      });

      expect(state.workspaces?.activeId).toBe("ws-1");
      expect(state.userSettings?.workspaceId).toBe("ws-1");
    });

    it("returns empty state defaults when all API calls fail", () => {
      const state = buildState({ userSettingsResponse: null });

      expect(state.workspaces).toEqual({ items: [], activeId: null });
      expect(state.executors).toEqual({ items: [] });
      expect(state.agentProfiles).toEqual({ items: [], version: 0 });
      expect(state.settingsAgents).toEqual({ items: [] });
      expect(state.agentDiscovery).toEqual({ items: [], loading: false, loaded: true });
      expect(state.availableAgents).toEqual({
        items: [],
        tools: [],
        loading: false,
        loaded: true,
      });
      expect(state.settingsData).toEqual({ executorsLoaded: true, agentsLoaded: true });
      expect(state.userSettings).toBeUndefined();
    });
  });

  it("only spreads userSettings when settings were loaded", () => {
    const loaded = buildState({
      workspaces: workspaceRows(["ws-1"]),
      userSettingsResponse: userSettings({ workspace_id: workspaceId("ws-1") }),
    });
    const failed = buildState({
      workspaces: workspaceRows(["ws-1"]),
      userSettingsResponse: null,
    });

    expect(loaded.userSettings?.loaded).toBe(true);
    expect(failed.userSettings).toBeUndefined();
  });
});

function buildState(
  overrides: Partial<Parameters<typeof buildSettingsInitialStateForRoute>[0]> = {},
) {
  return buildSettingsInitialStateForRoute({
    pathname: "/settings",
    workspaces: [],
    executors: [],
    agents: [],
    discoveryAgents: [],
    availableAgents: [],
    availableTools: [],
    userSettingsResponse: null,
    ...overrides,
  });
}

function buildWorkspace(
  params: Omit<
    Partial<ListWorkspacesResponse["workspaces"][number]>,
    "id" | "office_workflow_id"
  > & {
    id: string;
    office_workflow_id: ReturnType<typeof workflowId> | null;
  },
) {
  const { id, office_workflow_id, ...rest } = params;
  return {
    id: workspaceId(id),
    name: `Workspace ${id}`,
    description: null,
    owner_id: OWNER_ID,
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    office_workflow_id,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...rest,
  } as unknown as ListWorkspacesResponse["workspaces"][number];
}

function workspaceRows(ids: string[]): ListWorkspacesResponse["workspaces"] {
  return ids.map((id) => buildWorkspace({ id, office_workflow_id: null }));
}

function userSettings(
  settings: Partial<NonNullable<UserSettingsResponse["settings"]>>,
): UserSettingsResponse {
  return {
    settings: {
      user_id: OWNER_ID,
      workspace_id: workspaceId(""),
      workflow_filter_id: workflowId(""),
      repository_ids: [],
      updated_at: TIMESTAMP,
      ...settings,
    },
  };
}
